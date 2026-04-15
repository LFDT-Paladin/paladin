# Paladin Test CLI

Paladin Test CLI is an HTTP load testing tool that generates a constant request rate against a Paladin node and measures behavior under sustained load.

## Items Subject to Testing

- Public transaction submission
- Private transaction (pente) submission with node restart resilience testing
- Private transaction (pente) submission that creates a group and deploys a contract per run
- Noto submission with revertable hooks and mixed forced outcomes

## Build

The `pldtest` CLI needs building before you can use it.

Run `gradle build` in the `test` directory to build and install the `pldtest` command.

## Run

### Public contract

This test submits transactions which call the `set` method on a [`simplestorage`](https://github.com/kaleido-io/kaleido-js/blob/master/deploy-transact/contracts/simplestorage.sol) contract.

1. Create a configuration file for your test. See [`example-quick-start.yaml`](./config/example-quick-start.yaml) for an example and [`conf.go`](./internal/conf/conf.go) for all configuration options.
1. Deploy the `simplestorage` smart contract. Replace <key> in the following command with the the name of the key you wish to use, e.g. `key@node1`.
    ```
    curl --location '127.0.0.1:31548' \
    --header 'Content-Type: application/json' \
    --data-raw '{
        "jsonrpc": "2.0",
        "id": "1",
        "method": "ptx_sendTransaction",
        "params": [{
            "type": "public",
            "abi": [
                {
                    "inputs": [],
                    "stateMutability": "nonpayable",
                    "type": "constructor"
                },
                {
                    "anonymous": false,
                    "inputs": [
                        {
                            "indexed": false,
                            "internalType": "uint256",
                            "name": "data",
                            "type": "uint256"
                        }
                    ],
                    "name": "DataStored",
                    "type": "event"
                },
                {
                    "inputs": [],
                    "name": "get",
                    "outputs": [
                        {
                            "internalType": "uint256",
                            "name": "retVal",
                            "type": "uint256"
                        }
                    ],
                    "stateMutability": "view",
                    "type": "function"
                },
                {
                    "inputs": [],
                    "name": "query",
                    "outputs": [
                        {
                            "internalType": "uint256",
                            "name": "retVal",
                            "type": "uint256"
                        }
                    ],
                    "stateMutability": "view",
                    "type": "function"
                },
                {
                    "inputs": [
                        {
                            "internalType": "uint256",
                            "name": "x",
                            "type": "uint256"
                        }
                    ],
                    "name": "set",
                    "outputs": [
                        {
                            "internalType": "uint256",
                            "name": "value",
                            "type": "uint256"
                        }
                    ],
                    "stateMutability": "nonpayable",
                    "type": "function"
                },
                {
                    "inputs": [],
                    "name": "storedData",
                    "outputs": [
                        {
                            "internalType": "uint256",
                            "name": "",
                            "type": "uint256"
                        }
                    ],
                    "stateMutability": "view",
                    "type": "function"
                }
            ],
            "bytecode": "6080604052348015600e575f80fd5b5060015f819055506102af806100235f395ff3fe608060405234801561000f575f80fd5b506004361061004a575f3560e01c80632a1afcd91461004e5780632c46b2051461006c57806360fe47b11461008a5780636d4ce63c146100ba575b5f80fd5b6100566100d8565b604051610063919061018f565b60405180910390f35b6100746100dd565b604051610081919061018f565b60405180910390f35b6100a4600480360381019061009f91906101d6565b6100e5565b6040516100b1919061018f565b60405180910390f35b6100c261016f565b6040516100cf919061018f565b60405180910390f35b5f5481565b5f8054905090565b5f60648210610129576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016101209061025b565b60405180910390fd5b815f819055507f9455957c3b77d1d4ed071e2b469dd77e37fc5dfd3b4d44dc8a997cc97c7b3d498260405161015e919061018f565b60405180910390a15f549050919050565b5f8054905090565b5f819050919050565b61018981610177565b82525050565b5f6020820190506101a25f830184610180565b92915050565b5f80fd5b6101b581610177565b81146101bf575f80fd5b50565b5f813590506101d0816101ac565b92915050565b5f602082840312156101eb576101ea6101a8565b5b5f6101f8848285016101c2565b91505092915050565b5f82825260208201905092915050565b7f56616c75652063616e206e6f74206265206f76657220313030000000000000005f82015250565b5f610245601983610201565b915061025082610211565b602082019050919050565b5f6020820190508181035f83015261027281610239565b905091905056fea26469706673582212200f06afa0bff2e5cf52b2437330e8c116fbccbb884dc359663cd63af0a7712e5464736f6c634300081a0033",
            "function": "",
            "from": <key>,
            "data": {}
        }
        ]
    }'
    ```
1. Get the address the contract is deployed to and set contract address in the configuration file to this value
    ```
        curl --location '127.0.0.1:31548' \
    --header 'Content-Type: application/json' \
    --data '{
        "jsonrpc": "2.0",
        "id": "1",
        "method": "ptx_getTransactionFull",
        "params": [<transaction id from deploying contract>]
    }'
    ```
1. Create a receipt listener for public transactions. This listener must be called `publiclistener` and can be used across multiple test runs.
    ```
    curl --location '127.0.0.1:31548' \
    --header 'Content-Type: application/json' \
    --data '{
        "jsonrpc": "2.0",
        "id": "1",
        "method": "ptx_createReceiptListener",
        "params": [
            {
                "name": "publiclistener",
                "filters": {
                    "type": "public"
                }
            }
        ]
    }'
    ```
1. Run the test
    ```
    pldtest run -c <config file> -i 0
    ```

## Command line options

```
Usage:
  pldtest run [flags]

Flags:
  -c, --config string          Path to test config that describes the nodes and test instances
  -d, --daemon                 Run in long-lived, daemon mode. Any provided test length is ignored.
  -h, --help                   help for run
  -i, --instance-idx int       Index of the instance within test config to run (default -1)
  -n, --instance-name string   Instance within test config to run
```

## Metrics

The `pldtest` tool registers the following metrics for Prometheus to consume:

- `pldtest_runner_actions_submitted_total`
- `pldtest_runner_received_events_total`
- `pldtest_runner_incomplete_events_total`
- `pldtest_runner_perf_test_duration_seconds`

## Useful features

The `pldtest` tool is designed to let you run various styles of test. There are various options for creating your own customized tests. A full list of configuration options can be seen at [`conf.go`](internal/conf/conf.go) but some useful options are outlined below:

- Setting a maximum number of test actions
  - See `maxActions` attribute (defaults to `0` i.e. unlimited).
  - Once `maxActions` test actions (e.g. token mints) have taken place the test will shut down.
- Set the maximum duration of the test
  - See `length` attribute.
  - Setting a test instance's `length` attribute to a time duration (e.g. `3h`) will cause the test to run for that long or until an error occurs (the test exits on any worker error).
  - Note this setting is ignored if the test is run in daemon mode (running the `pldtest` command with `-d` or `--daemon`, or setting the global `daemon` value to `true` in the config). In daemon mode the test will run until `maxActions` has been reached or an error has occurred.
- Ramping up the rate of test actions
  - See the `startRate`, `endRate` and `rateRampUpTime` attribute of a test instance.
  - All values default to `0` which has the effect of not limiting the rate of the test.
  - The test will allow at most `startRate` actions to happen per second. Over the period of `rateRampUpTime` seconds the allowed rate will increase linearly until `endRate` actions per seconds are reached. At this point the test will continue at `endRate` actions per second until the test finishes.
  - If `startRate` is the only value that is set, the test will run at that rate for the entire test.
- Waiting for events to be confirmed before doing the next submission
  - See `noWaitSubmission` attribute (defaults to `false`).
  - When set to `false` (default), each worker routine will perform its action (e.g. minting a token) and wait for confirmation of that event before doing its next action.
  - When set to `true`, workers will submit actions without waiting for confirmations, allowing for higher throughput. This is useful for stress testing maximum submission rates.
  - `maxSubmissionsPerSecond` can be used to control the maximum number of submissions per second to avoid overloading the system under test.
- Having a worker loop submit more than 1 action per loop by setting `actionsPerLoop` for the test. This can be helpful when you want to scale the number of actions done in parallel without having to scale the number of workers. The default value is `1` for this attribute.

## Private Transaction Node Restart Test

This test drives pente (private) transactions at a configurable TPS across multiple nodes, kills a random node at regular intervals, waits for restart, and verifies all transactions complete successfully.

### Setup

1. Create a configuration file for your test. See [`example-private-transaction-node-restart.yaml`](./config/example-private-transaction-node-restart.yaml) for an example.
2. Configure multiple nodes in the `nodes` section of the config file. The privacy group members will be automatically generated as "member@" + each node name.
3. The contract bytecode and ABI are automatically loaded from `smart-contracts/simplestorage/simple_storage.json`.
4. Configure `nodeKillConfig` with:
   - `killCommandTemplate`: Command template to kill a node (uses Go template syntax with `.NodeName` field)
   - `healthCheckCommand`: Command template to check node health (uses Go template syntax with `.NodeName` field)
   - `healthCheckTemplate`: Go template to parse health check command output. The template receives `.Output` containing the command output and should output "true" if healthy, "false" otherwise.
   - `restartTimeout`: Maximum time to wait for node restart (default: 5m)
   - `killInterval`: Interval at which to kill a random node (e.g., 30s). When this interval elapses, a node is killed and the test waits for it to restart before resuming transaction submissions.
5. Set test parameters:
   - `maxSubmissionsPerSecond`: Maximum transactions per second (e.g., 10)
   - `completionTimeout`: Maximum time to wait for all transactions to complete after test ends (default: 5m)

### Running the Test

```bash
pldtest run -c <config file> -i 0
```

### Test Flow

1. **Setup Phase**: Creates a privacy group (with members automatically generated from node names) and deploys the SimpleStorage contract (loaded from `simple_storage.json`) to it
2. **Load Phase**: Workers submit pente transactions at the configured rate (`maxSubmissionsPerSecond`), randomly distributed across nodes
3. **Node Kill Phase**: At each `killInterval`, a random node is killed and new transaction submissions stop
4. **Recovery Phase**: The test waits for the killed node to restart (up to `restartTimeout`)
5. **Resume Phase**: Once the node restarts, transaction submissions resume
6. **Receipt Listener Behavior**: Receipt subscriptions stay active through node kill/restart; they are not explicitly unsubscribed/reconnected during the kill cycle
7. **Verification Phase**: When the test ends, it waits up to `completionTimeout` for all pending transactions to receive receipts

### Node Kill Configuration

The `killCommandTemplate` supports different deployment scenarios using Go template syntax:

- **Kubernetes**: `kubectl delete pod paladin-{{.NodeName}}-0 -n paladin --grace-period=0 --force`
- **Docker**: `docker kill {{.NodeName}}`
- **Custom**: Any command template using the `.NodeName` field

The `healthCheckCommand` and `healthCheckTemplate` work together to determine when a node has restarted:

- **Kubernetes example**: 
  - Command: `kubectl get pod paladin-{{.NodeName}}-0 -n paladin -o jsonpath='{.status.containerStatuses[*].ready}'`
  - Template: Checks that the output equals "true true" (both containers ready)
- **Custom**: The template receives `.Output` containing the command output and should output "true" if healthy, "false" otherwise

Available template fields:
- `.NodeName`: The name of the node from the configuration

## Privacy Group Contract Deploy Test

This test creates a new pente privacy group and deploys the `simple_storage` contract in each worker run.

### Setup

1. Create a configuration file for your test. See [`example-privacy-group-contract-deploy.yaml`](./config/example-privacy-group-contract-deploy.yaml) for an example.
2. Configure multiple nodes in the `nodes` section of the config file. The privacy group members are generated as `member@<node name>`.
3. Set test parameters:
   - `maxSubmissionsPerSecond`: Maximum submissions per second (for this test each submission creates a privacy group and deploys a contract)
   - `completionTimeout`: Maximum time to wait for receipts after test send phase ends
4. Do **not** configure `nodeKillConfig` for this test.

### Running the Test

```bash
pldtest run -c <config file> -i 0
```

### Test Flow

1. **Setup Phase**: Creates a pente private receipt listener with domain receipts enabled
2. **Worker Run Phase**: For each run, creates a new privacy group, waits for the group genesis receipt, then deploys `simple_storage`
3. **Correlation ID**: Uses the deploy transaction ID as the worker correlator so receipts map to the correct worker

## Noto Revertable Hooks Test

This test deploys a Noto domain configured with Pente hooks that can deliberately produce mixed outcomes during transfers (success, revert, fail, and optional invalid-input errors).

### Setup

1. Create a configuration file for your test. See [`example-noto-revertable-hooks.yaml`](./config/example-noto-revertable-hooks.yaml) for an example.
2. Configure exactly 3 nodes in the `nodes` section.
3. Set test parameters:
   - `maxSubmissionsPerSecond`: Maximum transfer submissions per second.
   - `completionTimeout`: Maximum time to wait for receipts after sends finish.
4. Configure optional test behavior under `test.options`:
   - `errorInterval`: Interval used to schedule forced error actions.
   - `includeInvalidInputErrors`: Whether `invalid_input` actions are included in the forced-error rotation.

### Running the Test

```bash
pldtest run -c <config file> -i 0
```

### Test Flow

1. **Setup Phase**: Deploys `RevertableTarget`, creates a Pente privacy group, deploys `NotoHooksRevertable`, deploys Noto, mints initial supply, and creates a Noto receipt listener.
2. **Load Phase**: Workers submit Noto `transfer` transactions.
3. **Forced Error Rotation**: On `errorInterval`, the suite queues the next forced action (`revert` -> `fail` -> optional `invalid_input`), while non-queued submissions are sent as `success`.
4. **Verification Phase**: Post-run validation checks receipt outcomes by bucket (`REVERT`, `FAIL`, optional `INVALID_INPUT`, and `SUCCESS`) and reports aggregated failures.
