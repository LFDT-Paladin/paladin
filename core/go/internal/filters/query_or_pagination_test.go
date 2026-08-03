package filters

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildQueryJSONOrPaginationCursor(t *testing.T) {
	var qf query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"limit": 10,
		"sort": [
			"blockNumber DESC",
			"transactionIndex DESC"
		],
		"or": [
			{
				"lessThan": [
					{
						"field": "blockNumber",
						"value": 143
					}
				]
			},
			{
				"equal": [
					{
						"field": "blockNumber",
						"value": 143
					}
				],
				"lessThan": [
					{
						"field": "transactionIndex",
						"value": 0
					}
				]
			}
		]
	}`), &qf)
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("indexed_transactions"), FieldMap{
			"blockNumber":      Int64Field("block_number"),
			"transactionIndex": Int64Field("transaction_index"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})

	t.Log(generatedSQL)
	assert.Contains(t, generatedSQL, "block_number < 143")
	assert.Contains(t, generatedSQL, "block_number = 143")
	assert.Contains(t, generatedSQL, "transaction_index < 0")
	assert.NotContains(t, generatedSQL, "block_number > 143")
}

func TestBuildQueryJSONOrPaginationCursorWithJoins(t *testing.T) {
	var qf query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"limit": 10,
		"sort": [
			"blockNumber DESC",
			"transactionIndex DESC"
		],
		"or": [
			{
				"lessThan": [
					{
						"field": "blockNumber",
						"value": 143
					}
				]
			},
			{
				"equal": [
					{
						"field": "blockNumber",
						"value": 143
					}
				],
				"lessThan": [
					{
						"field": "transactionIndex",
						"value": 0
					}
				]
			}
		]
	}`), &qf)
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		q := tx.Table("indexed_transactions").Joins("Block").
			Where(`EXISTS (SELECT 1 FROM transaction_receipts WHERE "transaction_receipts"."tx_hash" = "indexed_transactions"."hash")`)
		var results []*pldapi.IndexedTransaction
		db := BuildGORM(context.Background(), &qf, q, FieldMap{
			"blockNumber":      Int64Field("block_number"),
			"transactionIndex": Int64Field("transaction_index"),
		}).Find(&results)
		require.NoError(t, db.Error)
		return db
	})

	t.Log(generatedSQL)
	assert.Equal(t,
		`SELECT "indexed_transactions"."hash","indexed_transactions"."block_number","indexed_transactions"."transaction_index","indexed_transactions"."from","indexed_transactions"."to","indexed_transactions"."nonce","indexed_transactions"."contract_address","indexed_transactions"."result","Block"."number" AS "Block__number","Block"."hash" AS "Block__hash","Block"."timestamp" AS "Block__timestamp" FROM "indexed_transactions" LEFT JOIN "indexed_blocks" "Block" ON "indexed_transactions"."block_number" = "Block"."number" WHERE EXISTS (SELECT 1 FROM transaction_receipts WHERE "transaction_receipts"."tx_hash" = "indexed_transactions"."hash") AND ((EXISTS (SELECT 1 FROM transaction_receipts WHERE "transaction_receipts"."tx_hash" = "indexed_transactions"."hash") AND block_number < 143) OR (EXISTS (SELECT 1 FROM transaction_receipts WHERE "transaction_receipts"."tx_hash" = "indexed_transactions"."hash") AND block_number = 143 AND transaction_index < 0)) ORDER BY block_number DESC,transaction_index DESC LIMIT 10`,
		generatedSQL,
	)
}
