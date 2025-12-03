# Local Expense Tracker

A self-hosted financial aggregator written in Go. It syncs transactions from banks (via SimpleFIN) and shared expenses (via Splitwise), cleans them up with a Regex rule engine, and exports them to [Ledger-CLI](https://ledger-cli.org) / [Hledger](https://hledger.org) text format.

## Features
- 🏦 **Bank Sync:** Automated fetching via SimpleFIN Bridge.
- 🍕 **Splitwise Sync:** Imports shared expenses and calculates your specific share.
- 🤖 **Auto-Categorization:** Regex-based rule engine to tag transactions automatically.
- 📝 **Ledger Export:** Generates `main.journal` and monthly files automatically.
- 🖥️ **Web UI:** Local interface to map accounts and review/retag transactions.

## Setup

1. **Prerequisites**
   - Go 1.20+
   - A [SimpleFIN Bridge](https://bridge.simplefin.org/) account ($15/yr).
   - (Optional) A [Splitwise](https://secure.splitwise.com/apps) API Key.

2. **Configuration**
   Create a `.env` file:
   ```env
   PORT=8080
   SIMPLEFIN_ACCESS_TOKEN=https://<user>:<pass>@bridge.simplefin.org/simplefin/accounts
   SPLITWISE_API_KEY=your_splitwise_api_key
   LEDGER_FILE_PATH=./my_finances
   ```

3. **Run**
   ```bash
   # Install dependencies
   go mod tidy

   # Run the server
   go run .
   ```

4. **Usage**
   - Open `http://localhost:8080`.
   - Go to **Accounts** tab to map Bank Accounts -> Ledger Account names (e.g. `Assets:Checking`).
   - Go to **Auto-Rules** to set up Regex patterns (e.g. `^Uber` -> `Expenses:Transport`).
   - Go to **Transactions** to review and categorize.

## Reporting
This tool generates standard Ledger files. You can use any compatible tool to analyze your data.

**Using Hledger (CLI):**
```bash
# Balance Sheet
hledger -f my_transactions/main.journal bs

# Monthly Income Statement
hledger -f my_transactions/main.journal is --monthly

# Expenses by Category
hledger -f my_transactions/main.journal bal Expenses
```

**Using Hledger-Web (UI):**
```bash
hledger-web -f my_transactions/main.journal
```

**Using Fava:**
```bash
fava my_transactions/main.journal
```

## Import History
To backfill data older than 90 days (if supported by your bank):
```bash
pip install requests python-dotenv
python3 init_history.py
```

### 3. Fun things to try next (The "Cheatsheet")

Now that your data is in `my_transactions/main.journal`, try running these commands in your terminal (assuming you installed `hledger`):

**1. Where did my money go this month?**
```bash
hledger -f my_transactions/main.journal bal Expenses -p "this month" -S
```

**2. How much have I spent on Uber all time?**
```bash
hledger -f my_transactions/main.journal reg @Uber
```

**3. Show me a monthly bar chart of food spending:**
```bash
hledger -f my_transactions/main.journal reg Food --monthly --histogram
```