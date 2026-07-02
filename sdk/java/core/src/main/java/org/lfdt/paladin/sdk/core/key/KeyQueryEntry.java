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

/**
 * One node returned by {@code keymgr_queryKeys} — either a key (when {@link #isKey()}) or an
 * intermediate path node (when {@link #hasChildren()}). Immutable; mirrors {@code
 * pldapi.KeyQueryEntry}. The verifiers list is never {@code null}.
 */
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

  /**
   * Whether this node is a key (as opposed to an intermediate path node).
   *
   * @return {@code true} if this node is a key
   */
  @JsonProperty("isKey")
  public boolean isKey() {
    return isKey;
  }

  /**
   * Whether this node has child nodes beneath it.
   *
   * @return {@code true} if this node has children
   */
  @JsonProperty("hasChildren")
  public boolean hasChildren() {
    return hasChildren;
  }

  /**
   * The full path of this node's parent.
   *
   * @return the parent path
   */
  @JsonProperty("parent")
  public String parent() {
    return parent;
  }

  /**
   * The full path of this node.
   *
   * @return the node path
   */
  @JsonProperty("path")
  public String path() {
    return path;
  }

  /**
   * The name of this node within its parent.
   *
   * @return the node name
   */
  @JsonProperty("name")
  public String name() {
    return name;
  }

  /**
   * The zero-based index allocated to this node within its parent.
   *
   * @return the node index
   */
  @JsonProperty("index")
  public long index() {
    return index;
  }

  /**
   * The name of the wallet that holds the key, when this node is a key.
   *
   * @return the wallet name, or {@code null} if this node is not a key
   */
  @JsonProperty("wallet")
  public String wallet() {
    return wallet;
  }

  /**
   * The signing-module handle for the key, when this node is a key.
   *
   * @return the key handle, or {@code null} if this node is not a key
   */
  @JsonProperty("keyHandle")
  public String keyHandle() {
    return keyHandle;
  }

  /**
   * The verifiers resolved for the key, when this node is a key.
   *
   * @return the verifiers, never {@code null} (empty when unset)
   */
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
