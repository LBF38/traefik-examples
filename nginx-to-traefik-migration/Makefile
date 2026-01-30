# --- Variables ---
SHELL := /bin/bash
K3D_CLUSTER_NAME ?= mycluster
HELM ?= helm
KUBECTL ?= kubectl

# --- Core Targets ---
.PHONY: help
help:
	@echo "Usage: make [TARGET]"
	@echo ""
	@echo "Core:"
	@echo "  help                   Show this help message"
	@echo "  install-nginx          Install ingress-nginx via Helm"
	@echo "  install-traefik        Install Traefik via Helm"
	@echo "  delete-nginx           Uninstall ingress-nginx"
	@echo "  delete-traefik         Uninstall Traefik"
	@echo "  update_traefik_image   Uninstall Traefik"
	@echo ""
	@echo "Cluster Management:"
	@echo "  create-cluster     Create a k3d cluster"
	@echo "  delete-cluster     Delete the k3d cluster"
	@echo "  start-cluster      Start the k3d cluster"
	@echo "  stop-cluster       Stop the k3d cluster"
	@echo ""
	@echo "Variables:"
	@echo "  K3D_CLUSTER_NAME   Defines the name of the k3d cluster. (default = mycluster)"

# --- Ingress Controllers ---
.PHONY: install-nginx
install-nginx:
	# Add repo
	helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
	helm repo update

	# Install latest version of Kubernetes Ingress-NGINX
	helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
	  --namespace ingress-nginx --create-namespace \
	  --values nginx_values.yml

.PHONY: install-traefik
install-traefik:
	# Add Helm repo
	helm repo add traefik https://traefik.github.io/charts
	helm repo update

	# Install latest version of Traefik
	helm upgrade --install traefik traefik/traefik \
	  --namespace traefik --create-namespace \
	  --values traefik_values.yml


.PHONY: update traefik image
update_traefik_image:
	set -eu
	@echo "updating traefik image in k3d cluster..."
	docker tag docker.io/traefik/traefik:latest docker.io/traefik/traefik:v100.0.0
	k3d image import docker.io/traefik/traefik:v100.0.0 -c $(K3D_CLUSTER_NAME)
	@echo "current date: "
	date
	$(KUBECTL) delete -n traefik $$(kubectl get pods --selector "app.kubernetes.io/name=traefik" --output=name -n traefik)
	$(KUBECTL) logs -n traefik $$(kubectl get pods --selector "app.kubernetes.io/name=traefik" --output=name -n traefik)

.PHONY: delete-nginx
delete-nginx:
	$(HELM) uninstall ingress-nginx -n ingress-nginx

.PHONY: delete-traefik
delete-traefik:
	$(HELM) uninstall traefik -n traefik

# --- Cluster Management ---
.PHONY: create-cluster
create-cluster:
	k3d cluster create $(K3D_CLUSTER_NAME) -c k3d.yml

.PHONY: delete-cluster
delete-cluster:
	k3d cluster delete $(K3D_CLUSTER_NAME)

.PHONY: start-cluster
start-cluster:
	k3d cluster start $(K3D_CLUSTER_NAME)

.PHONY: stop-cluster
stop-cluster:
	k3d cluster stop $(K3D_CLUSTER_NAME)


# --- UTILS ---
.PHONY: generate-tls-certs
generate-tls-certs:
	@echo "Generating CA Certificate..."
	openssl genrsa -out ca.key 2048
	openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 -out ca.crt -subj "/CN=Testing CA"
	@echo "Generating server Certificates"
	openssl genrsa -out server.key 2048
	openssl req -new -key server.key -out server.csr -subj "/CN=your-domain.com"
	openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365 -sha256
	@echo "Generating client Certificates"
	openssl genrsa -out client.key 2048
	openssl req -new -key client.key -out client.csr -subj "/CN=Test Client"
	openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt -days 365 -sha256

.PHONY: k8s-tls-secret
k8s-tls-secret:
	$(KUBECTL) create secret tls tls-secret --cert=server.crt --key=server.key


.PHONY: canary-testing
canary-testing:
	for i in {1..100}; do curl -s http://production.localhost:9000 | grep "Name:"; done | sort | uniq -c
