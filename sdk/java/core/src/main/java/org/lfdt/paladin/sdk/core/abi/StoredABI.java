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
package org.lfdt.paladin.sdk.core.abi;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Collections;
import java.util.List;
import org.lfdt.paladin.sdk.core.types.Bytes32;

/**
 * The thin record automatically stored for any ABI used in a transaction — a deterministic hash of
 * the ABI together with the ABI itself. Immutable; mirrors {@code pldapi.StoredABI}. Returned by
 * {@code ptx_getStoredABI} and {@code ptx_queryStoredABIs}.
 */
@JsonPropertyOrder({"hash", "abi"})
public final class StoredABI {

  private final Bytes32 hash;
  private final List<AbiEntry> abi;

  @JsonCreator
  StoredABI(
      @JsonProperty("hash") final Bytes32 hash, @JsonProperty("abi") final List<AbiEntry> abi) {
    this.hash = hash;
    this.abi = abi == null ? Collections.emptyList() : List.copyOf(abi);
  }

  /**
   * The deterministic hash that identifies the stored ABI.
   *
   * @return the ABI hash, or {@code null} if unset
   */
  @JsonProperty("hash")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Bytes32 hash() {
    return hash;
  }

  /**
   * The ABI entries.
   *
   * @return the ABI entries, never {@code null} (empty when unset)
   */
  @JsonProperty("abi")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<AbiEntry> abi() {
    return abi;
  }

  @Override
  public String toString() {
    return "StoredABI{hash=" + hash + ", entries=" + abi.size() + "}";
  }
}
