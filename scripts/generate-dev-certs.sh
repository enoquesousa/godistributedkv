#!/bin/sh
# Generate a new, short-lived trust domain for the local Compose example.
set -eu
umask 077

cert_dir=${1:-certs/dev}
if [ "$#" -gt 1 ]; then
    printf 'Usage: %s [new-output-directory]\n' "$0" >&2
    exit 1
fi
if [ -e "$cert_dir" ] || [ -L "$cert_dir" ]; then
    printf 'Refusing to overwrite existing certificates: %s\n' "$cert_dir" >&2
    exit 1
fi
command -v openssl >/dev/null 2>&1 || {
    printf 'OpenSSL is required.\n' >&2
    exit 1
}
mkdir -p -- "$cert_dir"

openssl req -x509 -newkey rsa:2048 -sha256 -days 7 -nodes \
    -keyout "$cert_dir/ca.key" -out "$cert_dir/ca.crt" \
    -subj '/CN=godistributedkv local development CA' \
    -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' \
    -addext 'keyUsage=critical,keyCertSign,cRLSign'

for name in node1 node2 node3 client; do
    openssl req -new -newkey rsa:2048 -sha256 -nodes \
        -keyout "$cert_dir/$name.key" -out "$cert_dir/$name.csr" \
        -subj "/CN=$name"
    {
        printf '%s\n' 'basicConstraints=critical,CA:FALSE'
        printf '%s\n' 'keyUsage=critical,digitalSignature,keyEncipherment'
        if [ "$name" = client ]; then
            printf '%s\n' 'extendedKeyUsage=clientAuth'
        else
            printf '%s\n' 'extendedKeyUsage=serverAuth,clientAuth'
            printf 'subjectAltName=DNS:%s,DNS:localhost,IP:127.0.0.1,IP:::1\n' "$name"
        fi
    } > "$cert_dir/$name.ext"
    openssl x509 -req -in "$cert_dir/$name.csr" -sha256 -days 7 \
        -CA "$cert_dir/ca.crt" -CAkey "$cert_dir/ca.key" -CAcreateserial \
        -extfile "$cert_dir/$name.ext" -out "$cert_dir/$name.crt"
    rm -- "$cert_dir/$name.csr" "$cert_dir/$name.ext"
done

printf 'Development certificates created in %s (valid for 7 days).\n' "$cert_dir"
printf 'Keep ca.key offline; only mount ca.crt and each node\047s own certificate/key.\n'
