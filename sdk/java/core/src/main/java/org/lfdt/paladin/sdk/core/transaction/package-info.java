/*
 * Copyright contributors to Paladin, an LFDT project
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

/**
 * Transaction submission and receipt types for the Paladin Java SDK, mirroring {@code
 * pldapi/transaction.go}.
 *
 * <p>{@link org.lfdt.paladin.sdk.core.transaction.TransactionInput} is the body submitted when
 * sending a transaction (flattening Go's {@code TransactionBase} and {@code PublicTxOptions}), and
 * {@link org.lfdt.paladin.sdk.core.transaction.TransactionReceipt} is the result returned once a
 * transaction reaches a final state (flattening Go's {@code TransactionReceiptData} and its inlined
 * on-chain blocks). {@link org.lfdt.paladin.sdk.core.transaction.TransactionType} and {@link
 * org.lfdt.paladin.sdk.core.transaction.SubmitMode} are the associated enums.
 *
 * <p>All types are immutable and round-trip through any Jackson {@code ObjectMapper} with no
 * consumer configuration, following the {@code omitempty} conventions of the Go reference.
 */
package org.lfdt.paladin.sdk.core.transaction;
