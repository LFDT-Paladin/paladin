---
title: PrivacyGroupInput
---
{% include-markdown "./_includes/privacygroupinput_description.md" %}

### Example

```json
{
    "domain": "",
    "members": null,
    "name": ""
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `domain` | The domain of the privacy group | `string` |
| `members` | The member list must be a set of fully qualified identity locators 'some.identity@node.name' | `string[]` |
| `name` | Optional name for the privacy group, which is indexed for efficient query | `string` |
| `properties` | Application specific properties for the privacy group | `` |
| `configuration` | Domain specific configuration options that define the behavior of the privacy group | `` |
| `transactionOptions` | Options that will be propagated to the final private transaction that is submitted after the domain has validated the input properties and generated the base private transaction | [`PrivacyGroupTXOptions`](#privacygrouptxoptions) |

## PrivacyGroupTXOptions

| Field Name | Description | Type |
|------------|-------------|------|
| `idempotencyKey` | Idempotency key for the genesis transaction of the privacy group, unique across all transactions on this node. The group is only created along with its genesis transaction, so re-submitting with a key already in use does not create a group, and returns a JSON-RPC error with code -32001 and the ID of the existing transaction | `string` |
| `gas` | The gas limit for the transaction (optional) | [`HexUint64`](simpletypes.md#hexuint64) |
| `value` | The value transferred in the transaction (optional) | [`HexUint256`](simpletypes.md#hexuint256) |
| `maxPriorityFeePerGas` | The maximum priority fee per gas (optional) | [`HexUint256`](simpletypes.md#hexuint256) |
| `maxFeePerGas` | The maximum fee per gas (optional) | [`HexUint256`](simpletypes.md#hexuint256) |


