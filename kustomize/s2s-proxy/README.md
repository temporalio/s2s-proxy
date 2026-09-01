# s2s-proxy Kustomize Configuration

This directory contains the Kustomize configuration for deploying the `s2s-proxy` application. It was converted from the original Helm chart.

## Overview

Kustomize allows for declarative, template-free management of Kubernetes application configurations. This setup provides a `base` configuration and an example `overlay` to demonstrate how to customize the deployment for different environments or needs.

### Base Configuration (`base/`)

The `base/` directory contains the core Kubernetes manifests for `s2s-proxy`:
*   `configmap.yaml`: Contains the default application configuration (derived from `files/default.yaml` in the Helm chart).
*   `deployment.yaml`: Defines the `s2s-proxy` Deployment, including replica count, image, ports, probes, and volume mounts for the configuration.
*   `service.yaml`: Exposes the `s2s-proxy` application within the cluster.
*   `kustomization.yaml`: Lists the resources and can define common labels or other base-level Kustomize instructions.

The base aims to replicate the Helm chart deployed with its default values.

### Overlays (`overlays/`)

The `overlays/` directory is where you can define variations of the base configuration. We have provided an example `development` overlay.

#### Development Overlay (`overlays/development/`)

This overlay demonstrates common customization patterns:
*   **Resource Naming**: Adds a `dev-` prefix to all resources (e.g., `dev-s2s-proxy-deployment`).
*   **Image Customization**: Changes the image tag for the `s2s-proxy` container.
*   **Replica Count**: Modifies the number of replicas for the Deployment using a JSON patch.
*   **Configuration Override**: Patches the `s2s-proxy-config` ConfigMap using a strategic merge patch (`configmap-patch.yaml`). This is analogous to using `configOverride` in the Helm chart's `values.yaml`.

## How to Use

You will need `kubectl` (with Kustomize built-in, v1.14+) or a standalone `kustomize` CLI.

### View Rendered Manifests

To see the Kubernetes YAML that Kustomize will generate for a specific overlay (e.g., `development`):

```bash
# Using kubectl
kubectl kustomize overlays/development

# Or using standalone kustomize CLI
kustomize build overlays/development
```

To view the manifests for the base configuration:

```bash
# Using kubectl
kubectl kustomize base

# Or using standalone kustomize CLI
kustomize build base
```

### Apply to a Cluster

To apply the `development` overlay configuration to your Kubernetes cluster:

```bash
kubectl apply -k overlays/development
```

To apply the base configuration (generally less common for direct application, usually an overlay is applied):

```bash
kubectl apply -k base
```

### Delete from a Cluster

To delete the resources applied from the `development` overlay:

```bash
kubectl delete -k overlays/development
```

## Customizing Further

1.  **Create New Overlays**: Copy the `overlays/development` directory to a new directory (e.g., `overlays/production`) and modify its `kustomization.yaml` and patches as needed.
2.  **Modify Patches**:
    *   Adjust `images` entries in the overlay's `kustomization.yaml` to change container images or tags.
    *   Modify `patches` entries to change fields in the Deployment, Service, etc. JSON patches offer precise control.
    *   Update or create new strategic merge patch files (like `configmap-patch.yaml`) to alter ConfigMap data or other resource specifications.
3.  **Adjust the Base**: If a change should apply to all environments, consider modifying the manifests or `kustomization.yaml` in the `base/` directory. However, it's often preferable to keep the base minimal and apply all changes via overlays.

This Kustomize setup provides a flexible way to manage `s2s-proxy` deployments across different environments. 