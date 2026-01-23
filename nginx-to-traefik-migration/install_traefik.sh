#! /bin/bash

set -eu

# Add Helm repo
helm repo add traefik https://traefik.github.io/charts
helm repo update

# Install latest version of Traefik
helm upgrade --install traefik traefik/traefik \
  --namespace traefik --create-namespace \
  --values traefik_values.yml
