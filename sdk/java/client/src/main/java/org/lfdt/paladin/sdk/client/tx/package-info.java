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
 * The fluent transaction builder — the ergonomic front door for submitting transactions.
 *
 * <p>{@link org.lfdt.paladin.sdk.client.tx.TxBuilder} layers method chaining and deferred
 * validation over the raw {@code ptx_*} calls in {@link org.lfdt.paladin.sdk.client.ptx}, so a
 * transaction can be described in a single expression. Submitting and waiting are two steps, as in
 * the Go and TypeScript SDKs: {@code send()} returns a {@link
 * org.lfdt.paladin.sdk.client.tx.SentTransaction} handle as soon as the transaction is on its way,
 * and {@code waitForReceipt()} on that handle polls asynchronously for the receipt.
 */
package org.lfdt.paladin.sdk.client.tx;
