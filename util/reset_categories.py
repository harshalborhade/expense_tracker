import sqlite3
import os

DB_PATH = "C:\\hb\\code\\expense_tracker\\data\\ledger.db"

def main():
    if not os.path.exists(DB_PATH):
        print(f"[ERROR] Database not found at {DB_PATH}")
        return

    print("!!! WARNING !!!")
    print("This will reset ALL transaction categories to 'Expenses:Uncategorized'.")
    print("It will also mark them as 'Not Reviewed' so the AI can process them again.")
    print("This action cannot be undone.")
    
    confirm = input("Type 'RESET' to confirm: ")
    if confirm != "RESET":
        print("Aborted.")
        return

    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()

    print("[INFO] Resetting categories...")
    
    # Reset everything to Uncategorized and Unreviewed
    cursor.execute("""
        UPDATE transactions 
        SET ledger_category = 'Expenses:Uncategorized',
            is_reviewed = 0
    """)
    
    count = cursor.rowcount
    conn.commit()
    conn.close()
    
    print(f"[SUCCESS] Reset {count} transactions.")

if __name__ == "__main__":
    main()