import os
import sqlite3
import requests
import json
import time
from datetime import datetime, timedelta
from dotenv import load_dotenv

# --- Configuration ---
CHUNK_DAYS = 60 
HISTORY_DAYS = 365 * 2 # How far back to go total

def main():
    # 1. Load Config
    load_dotenv('../.env')
    access_url = os.getenv("SIMPLEFIN_ACCESS_TOKEN")
    if not access_url:
        print("[ERROR] SIMPLEFIN_ACCESS_TOKEN not found in .env")
        return

    if not access_url.endswith("/accounts"):
        access_url = access_url.rstrip("/") + "/accounts"

    # 2. Connect to Database
    db_path = "../data/ledger.db"
    if not os.path.exists(db_path):
        print(f"[ERROR] Database not found at {db_path}. Run your Go app once first.")
        return
    
    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()
    print(f"[OK] Connected to {db_path}")

    # 3. Calculate Date Ranges (Going Backwards)
    end_date = datetime.now()
    start_date = end_date - timedelta(days=HISTORY_DAYS)
    current_end = end_date

    print(f"[INFO] Starting Historical Import")
    print(f"   Target Window: {start_date.strftime('%Y-%m-%d')} -> {end_date.strftime('%Y-%m-%d')}")
    print("-" * 60)

    total_imported = 0

    while current_end > start_date:
        # Calculate current chunk
        current_start = current_end - timedelta(days=CHUNK_DAYS)
        if current_start < start_date:
            current_start = start_date

        # SimpleFIN requires Unix Timestamps
        ts_start = int(current_start.timestamp())
        ts_end = int(current_end.timestamp())
        
        start_str = current_start.strftime('%Y-%m-%d')
        end_str = current_end.strftime('%Y-%m-%d')

        print(f"\n[DATE] Preparing chunk: {start_str} to {end_str}")

        # --- PROMPT USER ---
        user_input = input(f"[PROMPT] Make request for this period? [Y/n/q]: ").strip().lower()
        if user_input == 'q':
            print("Quitting...")
            break
        if user_input == 'n':
            print("Skipping this chunk...")
            current_end = current_start
            continue

        try:
            # Prepare Request Data
            params = {
                "start-date": ts_start,
                "end-date": ts_end
            }
            
            # --- LOG REQUEST ---
            print(f"\n[REQUEST]:")
            # We mask the sensitive part of the URL for display
            masked_url = access_url.split('@')[-1] if '@' in access_url else "..."
            print(f"   URL: ...@{masked_url}")
            print(f"   Params: {params}")

            # Execute Request
            resp = requests.get(access_url, params=params)
            
            # --- LOG RESPONSE ---
            print(f"\n[RESPONSE] (Status: {resp.status_code}):")
            # Print first 500 chars of body to debug
            clean_body = resp.text.replace('\n', ' ')[:500] 
            print(f"   Body Preview: {clean_body}...")

            if resp.status_code != 200:
                print(f"[ERROR] API Error. Stopping.")
                break
            
            data = resp.json()
            
            # Process Transactions
            chunk_count = 0
            for account in data.get("accounts", []):
                account_id = account["id"]
                currency = account.get("currency", "USD")
                
                # Ensure Account Map
                cursor.execute("""
                    INSERT OR IGNORE INTO account_maps (external_id, provider, name, ledger_account)
                    VALUES (?, 'simplefin', ?, ?)
                """, (account_id, account["name"], f"Assets:FIXME:{account_id}"))

                # Process Tx
                txs = account.get("transactions", [])
                for t in txs:
                    tx_date = datetime.fromtimestamp(t["posted"]).strftime('%Y-%m-%d')
                    
                    cursor.execute("""
                        INSERT OR IGNORE INTO transactions 
                        (id, provider, account_id, date, payee, amount, currency, ledger_category, notes, is_reviewed)
                        VALUES (?, 'simplefin', ?, ?, ?, ?, ?, 'Expenses:Uncategorized', '', 0)
                    """, (
                        t["id"], 
                        account_id, 
                        tx_date, 
                        t["description"], 
                        float(t["amount"]), 
                        currency
                    ))
                    
                    if cursor.rowcount > 0:
                        chunk_count += 1

            conn.commit()
            print(f"[OK] Saved {chunk_count} new transactions from this chunk.")
            total_imported += chunk_count

        except Exception as e:
            print(f"[ERROR] Python Error: {e}")
            break

        # Move pointers
        current_end = current_start

    conn.close()
    print("-" * 60)
    print(f"[DONE] Import Finished. Total imported: {total_imported}")

if __name__ == "__main__":
    main()