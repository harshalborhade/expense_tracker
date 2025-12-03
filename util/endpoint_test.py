import requests

access_url = "https://7A0586EE5A87473E54FA450F2E8E8C362573C3E45AB387ACB7DC2981F671C09D:5BD74C55AD757F2A56B916E0D0E6BFC24C90B116E7080BBBE636E375B4C2D41F@beta-bridge.simplefin.org/simplefin/accounts"
response = requests.get(access_url)
print(response.json())