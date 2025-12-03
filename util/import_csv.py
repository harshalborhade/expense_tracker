import csv
import sqlite3
import sys
import hashlib
import os
from dateutil import parser

# --- Configuration ---
DB_PATH = "../data/ledger.db"

PROFILES = {
    "chase_checking": {
        "identifying_columns": ["Details", "Posting Date", "Check or Slip #"],
        "col_date": "Posting Date", "col_desc": "Description", "col_amt": "Amount", "invert_amount": False 
    },
    "chase_cc": {
        "identifying_columns": ["Transaction Date", "Post Date", "Category", "Memo"],
        "col_date": "Post Date", "col_desc": "Description", "col_amt": "Amount", "invert_amount": True
    },
    "sofi": {
        "identifying_columns": ["Date", "Description", "Type", "Current balance", "Status"],
        "col_date": "Date", "col_desc": "Description", "col_amt": "Amount", "invert_amount": False 
    },
    "amex": {
        "identifying_columns": ["Date", "Description", "Amount"],
        "match_exact": True,
        "col_date": "Date", "col_desc": "Description", "col_amt": "Amount", "invert_amount": True
    },
    "discover": {
        "identifying_columns": ["Trans. Date", "Post Date", "Category"],
        "col_date": "Post Date", "col_desc": "Description", "col_amt": "Amount", "invert_amount": True
    }
}

def get_db_connection():
    if not os.path.exists(DB_PATH):
        print(f"[ERROR] Database not found at {DB_PATH}")
        sys.exit(1)
    return sqlite3.connect(DB_PATH)

def list_accounts(cursor):
    print("\n--- Available Accounts ---")
    cursor.execute("SELECT external_id, name, provider FROM account_maps")
    for row in cursor.fetchall():
        print(f"ID: {row[0]} | Name: {row[1]} ({row[2]})")
    print("--------------------------")

def detect_profile(header_row):
    headers = [h.strip() for h in header_row]
    for name, rules in PROFILES.items():
        required = rules.get("identifying_columns", [])
        if rules.get("match_exact"):
            if len(headers) == len(required) and all(r in headers for r in required):
                return name, rules
        else:
            if all(r in headers for r in required):
                return name, rules
    return None, None

def clean_amount(amt_str):
    return float(amt_str.replace('$', '').replace(',', ''))

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 import_csv.py <path_to_csv>")
        sys.exit(1)

    file_path = sys.argv[1]
    conn = get_db_connection()
    cursor = conn.cursor()

    list_accounts(cursor)
    account_id = input("Enter Account ID: ").strip()

    # --- 1. Load CSV ---
    with open(file_path, 'r', encoding='utf-8-sig') as f:
        reader = csv.reader(f)
        try:
            headers = next(reader)
        except StopIteration:
            sys.exit("Empty file")
        
        profile_name, rules = detect_profile(headers)
        if not profile_name:
            print(f"[ERROR] Unknown CSV format. Headers: {headers}")
            sys.exit(1)
            
        print(f"[INFO] Detected Format: {profile_name}")
        
        f.seek(0)
        dict_reader = csv.DictReader(f)
        
        count = 0
        skipped = 0
        
        # --- 2. Track seen signatures to handle duplicates ---
        # Format: "date|desc|amount" -> count
        # Example: "2023-01-01|Starbucks|5.00" -> 0 (First time)
        #          "2023-01-01|Starbucks|5.00" -> 1 (Second time)
        seen_signatures = {} 

        print("[INFO] Processing...")
        
        for row in dict_reader:
            try:
                # Extract Data
                raw_date = row[rules["col_date"]]
                dt = parser.parse(raw_date)
                date_str = dt.strftime("%Y-%m-%d")
                
                desc = row[rules["col_desc"]].strip()
                
                raw_amt = row[rules["col_amt"]]
                if not raw_amt: continue
                
                amount = clean_amount(raw_amt)
                if rules["invert_amount"]:
                    amount = amount * -1

                # --- 3. Generate ID Logic ---
                # Create a signature for this transaction content
                signature = f"{date_str}|{desc}|{amount}"
                
                # Check how many times we've seen this exact signature in this file
                occurrence_count = seen_signatures.get(signature, 0)
                seen_signatures[signature] = occurrence_count + 1
                
                # Generate ID: hash(content + occurrence_count)
                # This ensures the 1st Starbucks coffee gets a different ID than the 2nd
                raw_string = f"{signature}|{occurrence_count}"
                tx_hash = hashlib.md5(raw_string.encode('utf-8')).hexdigest()
                tx_id = f"csv_{tx_hash}"

                # Insert
                cursor.execute("""
                    INSERT OR IGNORE INTO transactions 
                    (id, provider, account_id, date, payee, amount, currency, ledger_category, notes, is_reviewed)
                    VALUES (?, 'manual_csv', ?, ?, ?, ?, 'USD', 'Expenses:Uncategorized', 'CSV Import', 0)
                """, (tx_id, account_id, date_str, desc, amount))
                
                if cursor.rowcount > 0:
                    count += 1
                else:
                    skipped += 1
                    
            except Exception as e:
                print(f"[WARN] Skipped row: {e}")

        conn.commit()
        print(f"[SUCCESS] Imported {count} txns. Skipped {skipped} (already existed).")
        
    conn.close()

if __name__ == "__main__":
    main()