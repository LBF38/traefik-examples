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
	@echo "  help               Show this help message"
	@echo "  install-nginx      Install ingress-nginx via Helm"
	@echo "  install-traefik    Install Traefik via Helm"
	@echo "  delete-nginx       Uninstall ingress-nginx"
	@echo "  delete-traefik     Uninstall Traefik"
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
