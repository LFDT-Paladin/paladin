---
title: pstate_*
---
## `pstate_listSchemas`

### Parameters

0. `domain`: `string`

### Returns

0. `schemas`: [`Schema[]`](../types/schema.md#schema)

## `pstate_queryContractNullifiers`

### Parameters

0. `domain`: `string`
1. `contractAddress`: [`EthAddress`](../types/simpletypes.md#ethaddress)
2. `schemaRef`: [`Bytes32`](../types/simpletypes.md#bytes32)
3. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)
4. `qualifier`: [`StateStatusQualifier`](../types/statestatusqualifier.md#statestatusqualifier)

### Returns

0. `states`: [`State[]`](../types/state.md#state)

## `pstate_queryContractStates`

### Parameters

0. `domain`: `string`
1. `contractAddress`: [`EthAddress`](../types/simpletypes.md#ethaddress)
2. `schemaRef`: [`Bytes32`](../types/simpletypes.md#bytes32)
3. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)
4. `qualifier`: [`StateStatusQualifier`](../types/statestatusqualifier.md#statestatusqualifier)

### Returns

0. `states`: [`State[]`](../types/state.md#state)

## `pstate_queryNullifiers`

### Parameters

0. `domain`: `string`
1. `schemaRef`: [`Bytes32`](../types/simpletypes.md#bytes32)
2. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)
3. `qualifier`: [`StateStatusQualifier`](../types/statestatusqualifier.md#statestatusqualifier)

### Returns

0. `states`: [`State[]`](../types/state.md#state)

## `pstate_queryStates`

### Parameters

0. `domain`: `string`
1. `schemaRef`: [`Bytes32`](../types/simpletypes.md#bytes32)
2. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)
3. `qualifier`: [`StateStatusQualifier`](../types/statestatusqualifier.md#statestatusqualifier)

### Returns

0. `states`: [`State[]`](../types/state.md#state)

## `pstate_storeState`

### Parameters

0. `domain`: `string`
1. `contractAddress`: [`EthAddress`](../types/simpletypes.md#ethaddress)
2. `schemaRef`: [`Bytes32`](../types/simpletypes.md#bytes32)
3. `data`: [`RawJSON`](../types/simpletypes.md#rawjson)

### Returns

0. `state`: [`State`](../types/state.md#state)

## `pstate_transferPrivateState`

### Parameters

0. `domain`: `string`
1. `stateId`: [`HexBytes`](../types/simpletypes.md#hexbytes)
2. `recipient`: [`PrivateIdentityLocator`](../types/simpletypes.md#privateidentitylocator)

### Returns

0. `reliableMessageId`: [`UUID`](../types/simpletypes.md#uuid)

