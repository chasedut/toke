#!/bin/bash

# Generate a temporary password
TEMP_PASSWORD=$(openssl rand -base64 32)

# Create the .tauri directory if it doesn't exist
mkdir -p ~/.tauri

# Use expect to automate the password input
expect -c "
spawn npx tauri signer generate -w $HOME/.tauri/toke.key
expect \"Please enter a password to protect the secret key.\"
send \"$TEMP_PASSWORD\r\"
expect \"Please reenter the password to confirm:\"
send \"$TEMP_PASSWORD\r\"
expect eof
"

echo "Keys generated successfully!"
echo "Password: $TEMP_PASSWORD"
echo ""
echo "Public key:"
cat ~/.tauri/toke.key.pub
echo ""
echo "Save the password securely - you'll need it for signing updates!"