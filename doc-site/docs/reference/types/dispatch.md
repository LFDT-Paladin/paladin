---
title: Dispatch
---
{% include-markdown "./_includes/dispatch_description.md" %}

### Example

```json
{
    "id": "",
    "transactionID": "",
    "publicTransactionID": 0
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `id` | Unique identifier for the dispatch record | `string` |
| `transactionID` | The ID of the transaction that triggered this dispatch | `string` |
| `publicTransactionID` | Local database identifier of the public transaction created for this dispatch | `uint64` |

