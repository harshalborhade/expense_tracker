import sqlite3
import os
from datetime import datetime, timedelta

DB_PATH = "../data/ledger.db"
# Allow for date differences (e.g. you paid on Friday, it posted on Monday)
DATE_TOLERANCE_DAYS = 4 

def main():
    if not os.path.exists(DB_PATH):
        print(f"[ERROR] Database not found at {DB_PATH}")
        return

    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    cursor = conn.cursor()

    print("[INFO] Fetching Splitwise Settlements...")
    # Fetch all Splitwise Payments (Provider 'splitwise_payment')
    # We assume these are already set to 'Transfers:Splitwise' by the import script
    cursor.execute("""
        SELECT id, date, amount, payee 
        FROM transactions 
        WHERE provider = 'splitwise_payment'
        ORDER BY date DESC
    """)
    settlements = cursor.fetchall()
    print(f"      Found {len(settlements)} settlements to match.")

    print("[INFO] Scanning Bank Transactions for matches...")
    
    matches_found = 0

    for settle in settlements:
        settle_date = datetime.strptime(settle['date'], "%Y-%m-%d")
        settle_amt = float(settle['amount']) # Should be POSITIVE (e.g. 50.00)
        
        # We are looking for a Bank Transaction that is:
        # 1. Negative amount (Money Out) matching the settlement
        # 2. Date is within +/- tolerance
        # 3. Not already categorized as a Transfer (optional, but safer)
        
        target_amt = -settle_amt # Look for -50.00
        
        # Date Window
        start_date = (settle_date - timedelta(days=DATE_TOLERANCE_DAYS)).strftime("%Y-%m-%d")
        end_date = (settle_date + timedelta(days=DATE_TOLERANCE_DAYS)).strftime("%Y-%m-%d")

        # Find candidate in Bank Transactions (provider='simplefin' or 'manual_csv')
        cursor.execute("""
            SELECT id, date, payee, ledger_category
            FROM transactions 
            WHERE (provider = 'simplefin' OR provider = 'manual_csv')
              AND amount = ?
              AND date BETWEEN ? AND ?
              AND ledger_category != 'Transfers:Splitwise'
        """, (target_amt, start_date, end_date))
        
        candidates = cursor.fetchall()

        if len(candidates) == 1:
            # Perfect match found
            bank_tx = candidates[0]
            print(f"   [MATCH] Settle: {settle['date']} ${settle_amt} <--> Bank: {bank_tx['date']} ({bank_tx['payee']})")
            
            # Update Bank Transaction Category
            cursor.execute("""
                UPDATE transactions 
                SET ledger_category = 'Transfers:Splitwise', is_reviewed = 1 
                WHERE id = ?
            """, (bank_tx['id'],))
            
            matches_found += 1
            
        elif len(candidates) > 1:
            # Ambiguous match (e.g. two $50 payments on same day)
            # We skip auto-matching to be safe, or just pick the first one?
            # Let's pick the one with "Venmo" or "Zelle" in the name if possible.
            best_match = None
            for c in candidates:
                name = c['payee'].lower()
                if 'venmo' in name or 'zelle' in name or 'splitwise' in name:
                    best_match = c
                    break
            
            if best_match:
                print(f"   [FUZZY MATCH] Settle: {settle['date']} ${settle_amt} <--> Bank: {best_match['date']} ({best_match['payee']})")
                cursor.execute("""
                    UPDATE transactions 
                    SET ledger_category = 'Transfers:Splitwise', is_reviewed = 1 
                    WHERE id = ?
                """, (best_match['id'],))
                matches_found += 1
            else:
                print(f"   [SKIP] Ambiguous match for ${settle_amt} on {settle['date']}. Found {len(candidates)} candidates.")

    conn.commit()
    conn.close()
    
    print("-" * 50)
    print(f"[SUCCESS] Matched and updated {matches_found} transactions.")
    print("Run 'go run .' to regenerate ledger files.")

if __name__ == "__main__":
    main()