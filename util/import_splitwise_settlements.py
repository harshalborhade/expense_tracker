import os
import sqlite3
import requests
import json
import time
from datetime import datetime
from dotenv import load_dotenv

# --- Configuration ---
BATCH_SIZE = 50
DB_PATH = "../data/ledger.db"
TARGET_CATEGORY = "Transfers:Splitwise"

def main():
    load_dotenv('../.env')
    api_key = os.getenv("SPLITWISE_API_KEY")
    if not api_key:
        print("[ERROR] SPLITWISE_API_KEY not found in .env")
        return

    if not os.path.exists(DB_PATH):
        print(f"[ERROR] Database not found at {DB_PATH}.")
        return
    
    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()
    print(f"[OK] Connected to {DB_PATH}")

    headers = {"Authorization": f"Bearer {api_key}"}

    # 1. Get Current User ID
    print("[INFO] Authenticating...")
    resp = requests.get("https://secure.splitwise.com/api/v3.0/get_current_user", headers=headers)
    if resp.status_code != 200:
        print(f"[FATAL] Auth failed: {resp.status_code}")
        return
    my_id = resp.json()["user"]["id"]
    print(f"[OK] Logged in as User ID: {my_id}")

    # 2. Fetch Loop
    offset = 0
    total_imported = 0
    
    print(f"\n[INFO] Scanning for Settlements...")

    while True:
        print(f"[REQUEST] Fetching offset {offset}...")
        params = {
            "limit": BATCH_SIZE,
            "offset": offset,
            "dated_after": "1970-01-01T00:00:00Z"
        }
        
        resp = requests.get("https://secure.splitwise.com/api/v3.0/get_expenses", headers=headers, params=params)
        
        if resp.status_code != 200:
            print(f"[FATAL] API Error {resp.status_code}")
            break

        data = resp.json()
        expenses = data.get("expenses", [])
        if not expenses:
            break

        batch_count = 0
        
        for exp in expenses:
            # FILTER: We ONLY want payments (settlements)
            if not exp.get("payment"):
                continue
            
            if exp["deleted_at"] is not None:
                continue

            # Determine My Role
            amount = 0.0
            involved = False

            for u in exp["users"]:
                if u["user_id"] == my_id:
                    paid = float(u["paid_share"] or 0) # I paid X
                    owed = float(u["owed_share"] or 0) # I received X (technically owed to me)
                    
                    if paid > 0:
                        # I PAID money to settle up.
                        # This is POSITIVE for the Splitwise Ledger (Reduces Liability)
                        amount = paid
                        involved = True
                    elif owed > 0:
                        # I RECEIVED money.
                        # This is NEGATIVE for the Splitwise Ledger (Reduces the "Asset" of them owing me)
                        amount = -owed
                        involved = True

            if not involved:
                continue

            date_str = exp["date"][:10]
            tx_id = f"sw_{exp['id']}" # Standard ID format
            payee = exp["description"] # e.g. "Payment to Bob"

            # Insert
            cursor.execute("""
                INSERT OR IGNORE INTO transactions 
                (id, provider, account_id, date, payee, amount, currency, ledger_category, notes, is_reviewed)
                VALUES (?, 'splitwise_payment', 'splitwise_group', ?, ?, ?, ?, ?, 'Settlement Import', 1)
            """, (
                tx_id, 
                date_str, 
                payee, 
                amount, 
                exp["currency_code"],
                TARGET_CATEGORY
            ))

            if cursor.rowcount > 0:
                batch_count += 1

        conn.commit()
        if batch_count > 0:
            print(f"   [OK] Imported {batch_count} settlements.")
        
        total_imported += batch_count
        offset += BATCH_SIZE
        time.sleep(0.2)

    conn.close()
    print("-" * 50)
    print(f"[DONE] Total Settlements Imported: {total_imported}")

if __name__ == "__main__":
    main()