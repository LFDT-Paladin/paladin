---
title: ChainedDispatch
---
{% include-markdown "./_includes/chaineddispatch_description.md" %}

### Example

```json
{
    "id": "",
    "transactionID": "",
    "chainedTransactionID": ""
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `id` | Identifier for the chained dispatch record, correlates with sequencer activity subjectId for chained dispatches | `string` |
| `transactionID` | The original transaction that triggered this chained dispatch | `string` |
| `chainedTransactionID` | The transaction ID of the chained private transaction | `string` |

