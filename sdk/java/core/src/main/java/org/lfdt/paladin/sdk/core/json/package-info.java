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
 * JSON support for the Paladin Java SDK.
 *
 * <p>Holds {@link org.lfdt.paladin.sdk.core.json.PaladinObjectMapper}, the factory for the
 * SDK-wide Jackson {@code ObjectMapper} that bundles the cross-cutting serialization defaults
 * (Java time handling, lenient unknown-property policy, null omission, and big-number precision)
 * used by every higher-level module.
 */
package org.lfdt.paladin.sdk.core.json;
