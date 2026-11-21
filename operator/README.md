# Paladin Operator

## Description

The **Paladin Operator** is a Kubernetes operator that deploys and manages Paladin nodes. It creates the Besu and Paladin workloads, wires each Paladin node to its Besu node, and applies the smart-contract resources those nodes need.

**This file is the from-source path for operator and node development.** The commands below build the Paladin and operator images from this repository and load them into a local [Kind](https://kind.sigs.k8s.io/) cluster. Use them when you are changing the operator, the node, or the sample manifests.

If you only want a running 3-node Besu network and 3-node Paladin network from published images, use the [installation guide](https://LFDT-Paladin.github.io/paladin/head/getting-started/installation/). That guide is the complete install (Kind, CRDs, cert-manager, and the Helm chart in `devnet` mode). This README stays the short entry point for a source build so the two do not drift.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Hardware, Docker, and architecture](#hardware-docker-and-architecture)
- [Getting Started](#getting-started)
  - [One-Click Installation with Gradle](#one-click-installation-with-gradle)
  - [Manual Installation Steps](#manual-installation-steps)
    - [Step 1: Build the Paladin and Operator Images](#step-1-build-the-paladin-and-operator-images)
    - [Step 2: Start the Kind Cluster](#step-2-start-the-kind-cluster)
    - [Step 3: Load the Images into the Kind Cluster](#step-3-load-the-images-into-the-kind-cluster)
    - [Step 4: Install Helm CRDs](#step-4-install-helm-crds)
    - [Step 5: Install the Operator Using Helm](#step-5-install-the-operator-using-helm)
    - [Step 6: Verify the Operator is Running](#step-6-verify-the-operator-is-running)
    - [Step 7: Create a Functional Paladin Node](#step-7-create-a-functional-paladin-node)
    - [Step 8: Verify the Nodes are Running](#step-8-verify-the-nodes-are-running)
- [Cleanup](#cleanup)
- [Development](#development)
- [Additional Information](#additional-information)
- [Support and Contribution](#support-and-contribution)
- [License](#license)
- [Contact](#contact)

---

## Prerequisites

Before you begin, ensure you have the following installed:

- **Go** version **v1.26+** (matches `operator/go.mod` and the operator image)
- **Docker** version **17.03+**
- **Gradle** version **6.0+**
- **kubectl** version **v1.11.3+**
- **Kind** (Kubernetes in Docker) installed
- **Helm** version **v3+**
- **GNU Make**

The [troubleshooting guide](https://LFDT-Paladin.github.io/paladin/head/getting-started/troubleshooting/) calls for **Docker 27.0.0+** and **Kind 0.24.0+** when node ports on a local Kind cluster fail to connect. Older Docker and Kind combinations are a common cause of that failure.

---

## Hardware, Docker, and architecture

`make kind-start` creates the cluster in [`paladin-kind.yaml`](paladin-kind.yaml): one control-plane node and one worker. The sample in this guide then runs, in the `paladin` namespace:

- 1 operator pod. The chart limits that container to **500m CPU** and **128Mi** memory.
- 3 Besu pods.
- 3 Paladin pods. Each Paladin pod also runs a Postgres sidecar (`database.mode: sidecarPostgres` in the node samples).

Kind does not set memory limits on the Besu or Paladin containers, so they share the Docker VM with the image build. A from-source run compiles the Paladin image (JDK, Go, Node, and protoc) before the cluster starts, and the manual-install workflow deletes the Gradle cache and the Docker build cache before `kind` because those caches and the cluster compete for disk.

Practical allocation for this path on a laptop:

| Resource | Allocation |
| --- | --- |
| CPU | 4 cores available to Docker |
| Memory | 8 GiB is the low end and is easy to exhaust during the image build together with the 3+3 node sample. **16 GiB** for the Docker VM leaves headroom for both. |
| Disk | Room for the Paladin and operator images, the Kind node images, and Besu and Postgres. Build caches are large; prune them (`docker builder prune`) if Kind fails to start after a successful image build. |

**CPU architecture.** The Paladin and operator Dockerfiles build `linux/amd64` and `linux/arm64` (`TARGETARCH`). A local `../gradlew buildPaladinImage buildOperatorImage` produces the architecture of the Docker daemon: arm64 on Apple Silicon, amd64 on Intel and on the default CI runners. `make kind-promote` loads those images into Kind, so each image has to match the Kind node architecture.

Published images on GHCR are built for both architectures. The [installation guide](https://LFDT-Paladin.github.io/paladin/head/getting-started/installation/) pulls those and does not compile from this tree.

---

## Getting Started

> **Run all commands in this guide from the `operator/` directory, including `../gradlew deploy` and all `make` commands.**

From the repository root, change to the `operator/` directory:

```sh
cd operator/
```

### One-Click Installation with Gradle

For convenience, you can run the entire setup with a single Gradle command:

```sh
../gradlew deploy
```

This command performs all the necessary steps to deploy the Paladin Operator and create a functional Paladin node:

- **Builds** both the Paladin and Operator Docker images.
- **Starts** the Kind cluster.
- **Loads** the images into the cluster.
- **Installs** Helm CRDs.
- **Installs** the operator using Helm.
- **Verifies** the operator is running in the `paladin` namespace.
- **Creates** the Paladin nodes (Besu Node and Paladin Node).

**Note:** This is the quickest way to get everything up and running.

**Cleanup**

```
../gradlew clean
```

> This will not delete the cluster. Delete the cluster by running `make kind-delete`

---

### Manual Installation Steps

Alternatively, you can follow these steps to build and deploy the Paladin Operator and Paladin nodes in a local Kubernetes cluster using Kind.

#### Step 1: Build the Paladin and Operator Images

From the **operator directory**, run:

```sh
../gradlew buildPaladinImage buildOperatorImage
```

This command will:

- Build the **Paladin** Docker image.
- Build the **Operator** Docker image.

**Note:** The `docker` task in the Gradle build script orchestrates the building of both images.

#### Step 2: Start the Kind Cluster

Create a local Kubernetes cluster using Kind:

```sh
make kind-start
```

This command will create a Kind cluster named `paladin` (as specified in the `Makefile`).

#### Step 3: Load the Images into the Kind Cluster

Load the Paladin and Operator images into the Kind cluster:

```sh
make kind-promote
```

This command loads the necessary Docker images into the Kind cluster so they can be used by Kubernetes deployments.

#### Step 4: Install Helm CRDs

Install the Custom Resource Definitions (CRDs) required by the operator:

```sh
make install-crds
```

This uses Helm to install the CRDs defined in the operator's Helm chart.

#### Step 5: Install the Operator Using Helm

Deploy the Paladin Operator to the cluster:

```sh
make helm-install
```

This command will:

- Install the operator into the `paladin` namespace.
- Create the namespace if it doesn't exist.
- Set the operator's namespace appropriately.

#### Step 6: Verify the Operator is Running

Ensure the operator is running correctly in the `paladin` namespace:

```sh
kubectl get pods -n paladin
```

You should see output similar to:

```
NAME                                READY   STATUS    RESTARTS   AGE
paladin-operator-xxxxxxxxxx-xxxxx   1/1     Running   0          XXm
```

#### Step 7: Create a Functional Paladin Node

Now that the operator is running, you can create the Paladin nodes.

Generate the smart contract deployment samples that `config/samples` applies, then create 3 nodes (3 Besu nodes and 3 Paladin nodes):

```sh
../gradlew :operator:buildContractSamples
make create-node
```

This command will:

- Create a **Besu Genesis** ConfigMap.
- Create 3 **Besu Node** instances and related resources.
- Create 3 **Paladin Node** instances and related resources.

#### Step 8: Verify the Nodes are Running

Check the status of the nodes:

```sh
kubectl get pods -n paladin
```

You should see output similar to:

```
NAME                                READY   STATUS    RESTARTS   AGE
besu-node1-xxxxxxxxxx-xxxxx          1/1     Running   0          XXm
besu-node2-xxxxxxxxxx-xxxxx          1/1     Running   0          XXm
besu-node3-xxxxxxxxxx-xxxxx          1/1     Running   0          XXm
paladin-node1-xxxxxxxxxx-xxxxx       1/1     Running   0          XXm
paladin-node2-xxxxxxxxxx-xxxxx       1/1     Running   0          XXm
paladin-node3-xxxxxxxxxx-xxxxx       1/1     Running   0          XXm
```

This confirms that the Paladin and Besu nodes are successfully deployed and running.

---

## Cleanup

To remove the operator and clean up all resources from your cluster, run:

```sh
../gradlew clean
```

This command will:

- Delete all Paladin instances.
- Uninstall the operator Helm chart.
- Uninstall the CRDs.
- Clean up any build artifacts from your local environment.

**Note:** The `clean` task in the Gradle build script depends on the `cleanCluster` task, which invokes `make clean` to perform the cleanup.

---

## Development

### Building the Operator

To build the operator binary:

```sh
make build
```

### Running the Operator Locally

To run the operator locally (outside the cluster):

```sh
make run
```

This is useful for development and debugging purposes.

### Testing

Run unit tests:

```sh
make test
```

Run end-to-end tests:

```sh
make test-e2e
```

### Linting

Run linters to ensure code quality:

```sh
make lint
```

Automatically fix lint issues:

```sh
make lint-fix
```

---

## Additional Information

- **Operator SDK Documentation:** [Operator SDK](https://sdk.operatorframework.io/docs/)
- **Kubernetes Documentation:** [Kubernetes](https://kubernetes.io/docs/home/)
- **Kind Documentation:** [Kind](https://kind.sigs.k8s.io/docs/)
- **Helm Documentation:** [Helm](https://helm.sh/docs/)

---

## Support and Contribution

If you encounter any issues or have suggestions, please:

- **Open an Issue:** Submit a detailed issue on the project's issue tracker.
- **Submit a Pull Request:** Fork the repository, make your changes, and submit a pull request for review.

Contributions are welcome!

---

## License

This project is licensed under the **Apache License 2.0**.

---

## Contact

For questions or support, please contact the maintainers of this project.

---
 
