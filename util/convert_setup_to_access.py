import base64
import requests

SETUP_TOKEN = 'SETUP_TOKEN_PLACEHOLDER'

# Decode the setup token to get the claim URL
claim_url = base64.b64decode(SETUP_TOKEN).decode('utf-8')

# Make POST request to claim the access URL
response = requests.post(claim_url, headers={'Content-Length': '0'})
access_url = response.text

# Export access_url to a file
with open('access_url.txt', 'w') as f:
    f.write(access_url)

print(f"Access URL saved to access_url.txt")