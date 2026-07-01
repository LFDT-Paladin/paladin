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

// Immutable; mirrors pldapi.KeyQueryEntry. One node returned by keymgr_queryKeys — either a key
// (isKey) or an intermediate path node (hasChildren). The verifiers list is never null.
@JsonPropertyOrder({
  "isKey",
  "hasChildren",
  "parent",
  "path",
  "name",
  "index",
  "wallet",
  "keyHandle",
  "verifiers"
})
public final class KeyQueryEntry {

  private final boolean isKey;
  private final boolean hasChildren;
  private final String parent;
  private final String path;
  private final String name;
  private final long index;
  private final String wallet;
  private final String keyHandle;
  private final List<KeyVerifier> verifiers;

  @JsonCreator
  KeyQueryEntry(
      @JsonProperty("isKey") boolean isKey,
      @JsonProperty("hasChildren") boolean hasChildren,
      @JsonProperty("parent") String parent,
      @JsonProperty("path") String path,
      @JsonProperty("name") String name,
      @JsonProperty("index") long index,
      @JsonProperty("wallet") String wallet,
      @JsonProperty("keyHandle") String keyHandle,
      @JsonProperty("verifiers") List<KeyVerifier> verifiers) {
    this.isKey = isKey;
    this.hasChildren = hasChildren;
    this.parent = parent;
    this.path = path;
    this.name = name;
    this.index = index;
    this.wallet = wallet;
    this.keyHandle = keyHandle;
    this.verifiers = verifiers == null ? Collections.emptyList() : List.copyOf(verifiers);
  }

  @JsonProperty("isKey")
  public boolean isKey() {
    return isKey;
  }

  @JsonProperty("hasChildren")
  public boolean hasChildren() {
    return hasChildren;
  }

  @JsonProperty("parent")
  public String parent() {
    return parent;
  }

  @JsonProperty("path")
  public String path() {
    return path;
  }

  @JsonProperty("name")
  public String name() {
    return name;
  }

  @JsonProperty("index")
  public long index() {
    return index;
  }

  @JsonProperty("wallet")
  public String wallet() {
    return wallet;
  }

  @JsonProperty("keyHandle")
  public String keyHandle() {
    return keyHandle;
  }

  @JsonProperty("verifiers")
  public List<KeyVerifier> verifiers() {
    return verifiers;
  }

  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof KeyQueryEntry other
        && isKey == other.isKey
        && hasChildren == other.hasChildren
        && index == other.index
        && Objects.equals(parent, other.parent)
        && Objects.equals(path, other.path)
        && Objects.equals(name, other.name)
        && Objects.equals(wallet, other.wallet)
        && Objects.equals(keyHandle, other.keyHandle)
        && Objects.equals(verifiers, other.verifiers);
  }

  @Override
  public int hashCode() {
    return Objects.hash(
        isKey, hasChildren, parent, path, name, index, wallet, keyHandle, verifiers);
  }

  @Override
  public String toString() {
    return "KeyQueryEntry{path=" + path + ", isKey=" + isKey + ", hasChildren=" + hasChildren + "}";
  }
}
