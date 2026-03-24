---
title: ChainedTransaction
---
{% include-markdown "./_includes/chainedtransaction_description.md" %}

### Example

```json
{
    "chainedTransactionID": "",
    "transactionID": "",
    "localID": ""
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `chainedTransactionID` | The transaction ID of the chained private transaction | `string` |
| `transactionID` | The original transaction that triggered this chained transaction | `string` |
| `localID` | Local identifier for the chaining record, correlates with sequencer activity subjectId for chained dispatches | `string` |

