---
title: Dispatch
---
{% include-markdown "./_includes/dispatch_description.md" %}

### Example

```json
{
    "id": "",
    "privateTransactionID": "",
    "publicTransactionAddress": "0x0000000000000000000000000000000000000000",
    "publicTransactionID": 0
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `id` | Unique identifier for the dispatch record | `string` |
| `privateTransactionID` | The private transaction that triggered this dispatch | `string` |
| `publicTransactionAddress` | The signing address of the public transaction created for this dispatch | [`EthAddress`](simpletypes.md#ethaddress) |
| `publicTransactionID` | Local database identifier of the public transaction created for this dispatch | `uint64` |

