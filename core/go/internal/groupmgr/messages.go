/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package groupmgr

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

type persistedMessage struct {
	LocalSeq       uint64             `gorm:"column:local_seq;autoIncrement;primaryKey"`
	Domain         string             `gorm:"column:domain"`
	Group          pldtypes.HexBytes  `gorm:"column:group"`
	Node           string             `gorm:"column:node"`
	Sent           pldtypes.Timestamp `gorm:"column:sent"`
	Received       pldtypes.Timestamp `gorm:"column:received"`
	ID             uuid.UUID          `gorm:"column:id"`
	CID            *uuid.UUID         `gorm:"column:cid"`
	Topic          string             `gorm:"column:topic"`
	Data           pldtypes.RawJSON   `gorm:"column:data"`
	IdempotencyKey *string            `gorm:"column:idempotency_key"`
}

func (persistedMessage) TableName() string {
	return "pgroup_msgs"
}

var messageFilters = filters.FieldMap{
	"localSequence":  filters.Int64Field("local_seq"),
	"domain":         filters.StringField("domain"),
	"group":          filters.HexBytesField(`"group"`),
	"sent":           filters.TimestampField("sent"),
	"received":       filters.TimestampField("received"),
	"id":             filters.UUIDField("id"),
	"correlationId":  filters.UUIDField("cid"),
	"topic":          filters.StringField("topic"),
	"idempotencyKey": filters.StringField("idempotency_key"),
}

// Validation before attempting DB insertion
func (gm *persistedMessage) preValidate(ctx context.Context) error {
	if gm.Data == nil {
		return i18n.NewError(ctx, msgs.MsgPGroupsMessageDataNil)
	}
	if gm.Topic == "" {
		return i18n.NewError(ctx, msgs.MsgPGroupsMessageTopicEmpty)
	}
	// Check for invalid scenarios that could occur when receiving data from another node
	var zeroUUID uuid.UUID
	if gm.ID == zeroUUID || gm.Node == "" || gm.Sent == 0 || gm.Received == 0 || gm.Group == nil {
		log.L(ctx).Errorf("Invalid message: %+v", gm)
		return i18n.NewError(ctx, msgs.MsgPGroupsMessageInvalid)
	}
	return nil
}

func (gm *groupManager) SendMessage(ctx context.Context, dbTX persistence.DBTX, msg *pldapi.PrivacyGroupMessageInput) (_ *uuid.UUID, err error) {
	ctx = log.WithComponent(ctx, log.Component("groupmanager"))

	// An existing idempotencyKey takes precedence over any other failure, as re-validating a
	// message that has already been sent can fail where the original succeeded.
	if msg.IdempotencyKey != "" {
		defer func() {
			if err != nil {
				dbTX.AddPostRollback(func(txCtx context.Context, err error) error {
					// OUTSIDE of the rolled back transaction
					return gm.checkIdempotencyKey(ctx, err, msg.IdempotencyKey)
				})
			}
		}()
	}

	pg, err := gm.GetGroupByID(ctx, dbTX, msg.Domain, msg.Group)
	if err != nil {
		return nil, err
	}
	if pg == nil {
		return nil, i18n.NewError(ctx, msgs.MsgPGroupsGroupNotFound, msg.Group)
	}

	// Build and insert the message
	now := pldtypes.TimestampNow()
	msgID := uuid.New()
	pMsg := &persistedMessage{
		Domain:   msg.Domain,
		Group:    msg.Group,
		Sent:     now,
		Received: now,
		Node:     gm.transportManager.LocalNodeName(),
		ID:       msgID,
		CID:      msg.CorrelationID,
		Topic:    msg.Topic,
		Data:     msg.Data,
	}
	if msg.IdempotencyKey != "" {
		pMsg.IdempotencyKey = &msg.IdempotencyKey
	}
	if err := pMsg.preValidate(ctx); err != nil {
		return nil, err
	}
	if err := dbTX.DB(ctx).Create(pMsg).Error; err != nil {
		return nil, err
	}

	// Create the reliable message delivery to the other parties
	remoteMembers, err := gm.validateMembers(ctx, pg.Members, true)
	if err != nil {
		return nil, err
	}

	// We also need to create a reliable message to send the state to all the remote members
	msgs := make([]*pldapi.ReliableMessage, 0, len(remoteMembers))
	for node := range remoteMembers {
		// Each node gets a single copy (not one per identity)
		msgs = append(msgs, &pldapi.ReliableMessage{
			Node:        node,
			MessageType: pldapi.RMTPrivacyGroupMessage.Enum(),
			Metadata: pldtypes.JSONString(&components.PrivacyGroupMessageDistribution{
				Domain: msg.Domain,
				Group:  msg.Group,
				ID:     msgID,
			}),
		})
	}
	if len(msgs) > 0 {
		if err := gm.transportManager.SendReliable(ctx, dbTX, msgs...); err != nil {
			return nil, err
		}
	}

	dbTX.AddPostCommit(func(txCtx context.Context) {
		gm.notifyNewMessages([]*persistedMessage{pMsg})
	})

	return &msgID, nil

}

// Will either return the original error, or will return a special idempotency key error that can be used by the caller
// to determine that they need to ask for the existing message (rather than fail)
func (gm *groupManager) checkIdempotencyKey(ctx context.Context, origErr error, idempotencyKey string) error {
	existing, lookupErr := gm.GetMessageByIdempotencyKey(ctx, gm.p.NOTX(), idempotencyKey)
	if lookupErr != nil {
		log.L(ctx).Errorf("Failed to query for existing idempotencyKey after send error (returning original error): %s", lookupErr)
		return origErr
	}
	if existing != nil {
		log.L(ctx).Errorf("Overriding send error with idempotencyKey error. origErr: %s", origErr)
		return i18n.NewError(ctx, msgs.MsgPGroupsIdempotencyKeyClash, existing.ID)
	}
	return origErr
}

func (gm *groupManager) ReceiveMessages(ctx context.Context, dbTX persistence.DBTX, messages []*pldapi.PrivacyGroupMessage) (results map[uuid.UUID]error, err error) {
	ctx = log.WithComponent(ctx, log.Component("groupmanager"))
	results = make(map[uuid.UUID]error)
	now := pldtypes.TimestampNow()
	pMsgs := make([]*persistedMessage, 0, len(messages))
	validatedGroups := make(map[string]*pldapi.PrivacyGroup)
	for _, msg := range messages {
		pm := &persistedMessage{
			Domain:   msg.Domain,
			Group:    msg.Group,
			Sent:     msg.Sent,
			Received: now,      // we're receiving
			Node:     msg.Node, // must be validated by caller
			ID:       msg.ID,
			CID:      msg.CorrelationID,
			Topic:    msg.Topic,
			Data:     msg.Data,
		}
		if err := pm.preValidate(ctx); err != nil {
			log.L(ctx).Errorf("Unable to process received message %s: %s", pm.ID, err)
			results[pm.ID] = err
			continue
		}
		mapKey := pm.Domain + "/" + pm.Group.String()
		if validatedGroups[mapKey] == nil {
			group, err := gm.GetGroupByID(ctx, dbTX, pm.Domain, pm.Group)
			if err != nil {
				return nil, err
			}
			if group == nil {
				log.L(ctx).Errorf("Unable to process received message as group not initialized %s: %s", pm.ID, err)
				results[pm.ID] = i18n.NewError(ctx, msgs.MsgPGroupsGroupNotFound, pm.Group)
				continue
			}
			validatedGroups[mapKey] = group
		}
		results[pm.ID] = nil // success
		pMsgs = append(pMsgs, pm)
	}

	if len(pMsgs) > 0 {
		if err := dbTX.DB(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(pMsgs).
			Error; err != nil {
			return nil, err
		}

		dbTX.AddPostCommit(func(txCtx context.Context) {
			gm.notifyNewMessages(pMsgs)
		})
	}

	return results, nil
}

func (gm *groupManager) QueryMessages(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PrivacyGroupMessage, error) {
	ctx = log.WithComponent(ctx, log.Component("groupmanager"))
	qw := &filters.QueryWrapper[persistedMessage, pldapi.PrivacyGroupMessage]{
		P:           gm.p,
		DefaultSort: "-localSequence",
		Filters:     messageFilters,
		Query:       jq,
		MapResult: func(dbPM *persistedMessage) (*pldapi.PrivacyGroupMessage, error) {
			return dbPM.mapToAPI(), nil
		},
	}
	return qw.Run(ctx, dbTX)
}

func (gm *groupManager) GetMessageByID(ctx context.Context, dbTX persistence.DBTX, id uuid.UUID, failNotFound bool) (*pldapi.PrivacyGroupMessage, error) {
	ctx = log.WithComponent(ctx, log.Component("groupmanager"))
	dbMsgs, err := gm.QueryMessages(ctx, dbTX, query.NewQueryBuilder().Equal("id", id).Limit(1).Query())
	if err != nil {
		return nil, err
	}
	if len(dbMsgs) < 1 {
		if failNotFound {
			return nil, i18n.NewError(ctx, msgs.MsgPGroupsMessageNotFound)
		}
		return nil, nil
	}
	return dbMsgs[0], nil
}

func (gm *groupManager) GetMessageForDistribution(ctx context.Context, dbTX persistence.DBTX, id uuid.UUID) (*pldapi.PrivacyGroupMessage, error) {
	msg, err := gm.GetMessageByID(ctx, dbTX, id, false)
	if msg != nil {
		// The idempotencyKey is local to the sending node
		msg.IdempotencyKey = ""
	}
	return msg, err
}

func (gm *groupManager) GetMessageByIdempotencyKey(ctx context.Context, dbTX persistence.DBTX, idempotencyKey string) (*pldapi.PrivacyGroupMessage, error) {
	ctx = log.WithComponent(ctx, log.Component("groupmanager"))
	dbMsgs, err := gm.QueryMessages(ctx, dbTX, query.NewQueryBuilder().Equal("idempotencyKey", idempotencyKey).Limit(1).Query())
	if len(dbMsgs) == 0 || err != nil {
		return nil, err
	}
	return dbMsgs[0], nil
}
