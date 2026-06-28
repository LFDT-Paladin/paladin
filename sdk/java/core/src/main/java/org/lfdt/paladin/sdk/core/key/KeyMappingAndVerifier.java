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

package org.lfdt.paladin.sdk.core.key;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Collections;
import java.util.List;
import java.util.Objects;

// Immutable; mirrors pldapi.KeyMappingAndVerifier (which inlines KeyMappingWithPath -> KeyMapping).
// The embedded mapping fields (identifier, wallet, keyHandle, path) are flattened here to match the
// flat JSON wire form. The path list is never null (empty when unset).
@JsonPropertyOrder({"identifier", "wallet", "keyHandle", "path", "verifier"})
public final class KeyMappingAndVerifier {

  private final String identifier;
  private final String wallet;
  private final String keyHandle;
  private final List<KeyPathSegment> path;
  private final KeyVerifier verifier;

  @JsonCreator
  KeyMappingAndVerifier(
      @JsonProperty("identifier") String identifier,
      @JsonProperty("wallet") String wallet,
      @JsonProperty("keyHandle") String keyHandle,
      @JsonProperty("path") List<KeyPathSegment> path,
      @JsonProperty("verifier") KeyVerifier verifier) {
    this.identifier = identifier;
    this.wallet = wallet;
    this.keyHandle = keyHandle;
    this.path = path == null ? Collections.emptyList() : List.copyOf(path);
    this.verifier = verifier;
  }

  @JsonProperty("identifier")
  public String identifier() {
    return identifier;
  }

  @JsonProperty("wallet")
  public String wallet() {
    return wallet;
  }

  @JsonProperty("keyHandle")
  public String keyHandle() {
    return keyHandle;
  }

  @JsonProperty("path")
  public List<KeyPathSegment> path() {
    return path;
  }

  @JsonProperty("verifier")
  public KeyVerifier verifier() {
    return verifier;
  }

  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof KeyMappingAndVerifier other
        && Objects.equals(identifier, other.identifier)
        && Objects.equals(wallet, other.wallet)
        && Objects.equals(keyHandle, other.keyHandle)
        && Objects.equals(path, other.path)
        && Objects.equals(verifier, other.verifier);
  }

  @Override
  public int hashCode() {
    return Objects.hash(identifier, wallet, keyHandle, path, verifier);
  }

  @Override
  public String toString() {
    return "KeyMappingAndVerifier{identifier=" + identifier + ", wallet=" + wallet + ", verifier="
        + verifier + "}";
  }
}
