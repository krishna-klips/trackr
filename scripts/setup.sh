#!/bin/bash
# scripts/setup.sh

set -e

echo "=== Trackr Setup ==="

# Create directories
mkdir -p /var/lib/trackr/dbs
mkdir -p /var/lib/trackr/geoip
mkdir -p /var/log/trackr
mkdir -p /etc/trackr/saml

# Download GeoIP database (Need License Key)
if [ ! -z "$MAXMIND_LICENSE_KEY" ]; then
    echo "Downloading GeoIP database..."
    wget -O /var/lib/trackr/geoip/GeoLite2-City.mmdb.tar.gz \
      "https://download.maxmind.com/app/geoip_download?license_key=${MAXMIND_LICENSE_KEY}&edition_id=GeoLite2-City&suffix=tar.gz"
    tar -xzf /var/lib/trackr/geoip/GeoLite2-City.mmdb.tar.gz -C /var/lib/trackr/geoip --strip-components=1
else
    echo "MAXMIND_LICENSE_KEY not set, skipping GeoIP download."
fi

# Generate SAML certificates
if [ ! -f /etc/trackr/saml/sp-key.pem ]; then
    echo "Generating SAML certificates..."
    openssl req -x509 -newkey rsa:2048 -keyout /etc/trackr/saml/sp-key.pem \
      -out /etc/trackr/saml/sp-cert.pem -days 3650 -nodes \
      -subj "/CN=trackr.io"
fi

# Run database migrations
echo "Running global DB migrations..."
# Assuming trackr binary is built and in path or ./bin
if [ -f ./bin/trackr-migrate ]; then
    ./bin/trackr-migrate --target=global --direction=up
fi

echo "Setup complete!"
