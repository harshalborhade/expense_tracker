import os
import sqlite3
import requests
import json
import time
import sys
from datetime import datetime
from dotenv import load_dotenv

# --- Configuration ---
BATCH_SIZE = 50 # Reduced slightly to be safer and give more frequent updates
DB_PATH = "../data/ledger.db"

def main():
    # 1. Load Config
    load_dotenv('../.env')
    api_key = os.getenv("SPLITWISE_API_KEY")
    if not api_key:
        print("[ERROR] SPLITWISE_API_KEY not found in .env")
        return

    # 2. Connect to Database
    if not os.path.exists(DB_PATH):
        print(f"[ERROR] Database not found at {DB_PATH}.")
        return
    
    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()
    print(f"[OK] Connected to {DB_PATH}")

    headers = {"Authorization": f"Bearer {api_key}"}

    # 3. Get Current User ID
    print("[INFO] Authenticating...")
    try:
        resp = requests.get("https://secure.splitwise.com/api/v3.0/get_current_user", headers=headers)
        if resp.status_code != 200:
            print(f"[FATAL] Auth failed: {resp.status_code} - {resp.text}")
            return
        my_id = resp.json()["user"]["id"]
        print(f"[OK] Logged in as User ID: {my_id}")
    except Exception as e:
        print(f"[FATAL] Connection error during auth: {e}")
        return

    # 4. Ensure Account Map Exists
    cursor.execute("""
        INSERT OR IGNORE INTO account_maps (external_id, provider, name, ledger_account)
        VALUES ('splitwise_group', 'splitwise', 'Splitwise Shared Expenses', 'Liabilities:Payable:Splitwise')
    """)
    conn.commit()

    # 5. Fetch Loop
    offset = 0
    total_imported = 0
    
    print(f"\n[INFO] Starting Import (Batch Size: {BATCH_SIZE})")
    print("-" * 60)

    try:
        while True:
            print(f"[REQUEST] Fetching offset {offset}...")
            
            params = {
                "limit": BATCH_SIZE,
                "offset": offset,
                "dated_after": "1970-01-01T00:00:00Z"
            }

            resp = requests.get("https://secure.splitwise.com/api/v3.0/get_expenses", headers=headers, params=params)
            
            # --- FAIL SAFE: Stop on Error ---
            if resp.status_code != 200:
                print(f"\n[FATAL] API Error {resp.status_code}")
                print(f"Server Message: {resp.text}")
                print("Stopping import to prevent issues.")
                break

            data = resp.json()
            expenses = data.get("expenses", [])
            
            if not expenses:
                print("[INFO] No more expenses returned by API.")
                break

            # --- Verbose Date Context ---
            # Splitwise returns newest first. 
            # expenses[0] is the newest in this batch.
            # expenses[-1] is the oldest in this batch.
            newest_date = expenses[0].get("date", "???")[:10]
            oldest_date = expenses[-1].get("date", "???")[:10]
            print(f"   -> Batch covers range: {newest_date} to {oldest_date}")

            batch_count = 0
            
            for exp in expenses:
                if exp["deleted_at"] is not None:
                    continue

                # Calc Share
                my_share = 0.0
                did_i_pay = False
                for u in exp["users"]:
                    if u["user_id"] == my_id:
                        owed = float(u["owed_share"] or 0)
                        paid = float(u["paid_share"] or 0)
                        if owed > 0: my_share = -owed
                        if paid > 0: did_i_pay = True

                if my_share == 0: continue

                # Parse Date
                date_str = exp["date"][:10] # Safe slice for YYYY-MM-DD

                provider_label = "splitwise"
                if did_i_pay: provider_label = "splitwise_payer"

                tx_id = f"sw_{exp['id']}"

                cursor.execute("""
                    INSERT OR IGNORE INTO transactions 
                    (id, provider, account_id, date, payee, amount, currency, ledger_category, notes, is_reviewed)
                    VALUES (?, ?, 'splitwise_group', ?, ?, ?, ?, 'Expenses:Uncategorized', ?, 0)
                """, (
                    tx_id, 
                    provider_label, 
                    date_str, 
                    exp["description"], 
                    my_share, 
                    exp["currency_code"],
                    "Splitwise Import"
                ))

                if cursor.rowcount > 0:
                    batch_count += 1

            conn.commit()
            print(f"   [OK] Saved {batch_count} new entries.")
            
            total_imported += batch_count
            offset += BATCH_SIZE
            
            # Gentle rate limiting
            time.sleep(0.5)

    except KeyboardInterrupt:
        print("\n[WARN] User interrupted the script. Saving and exiting...")
    except Exception as e:
        print(f"\n[FATAL] Unexpected error: {e}")
    finally:
        conn.close()
        print("-" * 60)
        print(f"[DONE] Import Session Finished.")
        print(f"       Total New Transactions: {total_imported}")

if __name__ == "__main__":
    main()