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
 * <p>{@link org.lfdt.paladin.sdk.client.tx.TxBuilder} layers method chaining, deferred validation
 * and asynchronous receipt polling over the raw {@code ptx_*} calls in {@link
 * org.lfdt.paladin.sdk.client.ptx}, so a transaction can be described and awaited in a single
 * expression.
 */
package org.lfdt.paladin.sdk.client.tx;
