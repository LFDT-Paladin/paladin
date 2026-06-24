# Stage 1: Compile Solidity contracts to generate ABI artefacts
FROM node:20-bookworm-slim AS solidity-builder
WORKDIR /workspace
COPY solidity/package.json solidity/package-lock.json solidity/
COPY solidity/contracts solidity/contracts
COPY solidity/hardhat.config.ts solidity/
COPY solidity/tsconfig.json solidity/
RUN cd solidity && npm ci && npm run compile

# Stage 2: Build the pldtest binary
FROM golang:1.24-bookworm AS go-builder
WORKDIR /workspace

COPY go.work go.work.sum ./
COPY test/ test/
COPY sdk/go/ sdk/go/
COPY config/ config/
COPY common/go/ common/go/
COPY toolkit/go/ toolkit/go/
# go.work references all workspace modules; copy go.mod stubs for those not otherwise needed
COPY core/go/go.mod core/go/go.mod
COPY domains/integration-test/go.mod domains/integration-test/go.mod
COPY domains/noto/go.mod domains/noto/go.mod
COPY domains/zeto/go.mod domains/zeto/go.mod
COPY operator/go.mod operator/go.mod
COPY registries/evm/go.mod registries/evm/go.mod
COPY registries/static/go.mod registries/static/go.mod
COPY rpcauth/basicauth/go.mod rpcauth/basicauth/go.mod
COPY signingmodules/example/go.mod signingmodules/example/go.mod
COPY testinfra/go.mod testinfra/go.mod
COPY transports/grpc/go.mod transports/grpc/go.mod

# Populate the abis directory from the compiled Solidity artefacts
RUN mkdir -p test/internal/contracts/abis
COPY --from=solidity-builder /workspace/solidity/artifacts/contracts/testcontracts/SimpleStorage.sol/SimpleStorage.json test/internal/contracts/abis/
COPY --from=solidity-builder /workspace/solidity/artifacts/contracts/testcontracts/RevertableTarget.sol/RevertableTarget.json test/internal/contracts/abis/
COPY --from=solidity-builder /workspace/solidity/artifacts/contracts/testcontracts/NotoHooksRevertable.sol/NotoHooksRevertable.json test/internal/contracts/abis/
COPY --from=solidity-builder /workspace/solidity/artifacts/contracts/private/NotoTrackerERC20.sol/NotoTrackerERC20.json test/internal/contracts/abis/

ARG BUILD_VERSION=canary
ARG BUILD_DATE=unknown
ARG BUILD_COMMIT=unknown
RUN cd test && go build \
    -ldflags="-X 'github.com/LFDT-Paladin/paladin/test/internal/version.Version=${BUILD_VERSION}' -X 'github.com/LFDT-Paladin/paladin/test/internal/version.Date=${BUILD_DATE}' -X 'github.com/LFDT-Paladin/paladin/test/internal/version.Commit=${BUILD_COMMIT}'" \
    -o /pldtest \
    ./pldtest/

# Stage 3: Runtime image
FROM ubuntu:24.04

ARG TARGETARCH

RUN apt-get update && apt-get install -y curl && apt-get clean

# Install kubectl
RUN ARCH=$(dpkg --print-architecture) && \
    curl -LO "https://dl.k8s.io/release/$(curl -Ls https://dl.k8s.io/release/stable.txt)/bin/linux/${ARCH}/kubectl" && \
    install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl && \
    rm kubectl

COPY --from=go-builder /pldtest /usr/local/bin/pldtest

USER 65532:65532

ENTRYPOINT ["pldtest"]
