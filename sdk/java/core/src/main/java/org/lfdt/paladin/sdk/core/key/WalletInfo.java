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
import java.util.Objects;

/**
 * One wallet configured on the node, as returned by {@code keymgr_wallets}.
 *
 * <p>A wallet claims key identifiers by matching them against its key selector, a regular
 * expression. When {@link #keySelectorMustNotMatch()} is set the sense is inverted: the wallet
 * claims identifiers that do <em>not</em> match. Immutable.
 *
 * <p>Selectors are evaluated on the node against <a href="https://golang.org/s/re2syntax">RE2
 * syntax</a>, not {@link java.util.regex}. RE2 has no backreferences ({@code \1}) and no lookahead
 * or lookbehind ({@code (?=...)}, {@code (?<!...)}), so a selector using them is rejected by the
 * node even though it would compile as a Java pattern.
 */
@JsonPropertyOrder({"name", "keySelector", "keySelectorMustNotMatch"})
public final class WalletInfo {

  private final String name;
  private final String keySelector;
  private final boolean keySelectorMustNotMatch;

  @JsonCreator
  WalletInfo(
      @JsonProperty("name") final String name,
      @JsonProperty("keySelector") final String keySelector,
      @JsonProperty("keySelectorMustNotMatch") final boolean keySelectorMustNotMatch) {
    this.name = name;
    this.keySelector = keySelector;
    this.keySelectorMustNotMatch = keySelectorMustNotMatch;
  }

  /**
   * The name of the wallet.
   *
   * @return the wallet name
   */
  @JsonProperty("name")
  public String name() {
    return name;
  }

  /**
   * The regular expression used to select which key identifiers this wallet holds, in <a
   * href="https://golang.org/s/re2syntax">RE2 syntax</a>.
   *
   * @return the key selector expression
   */
  @JsonProperty("keySelector")
  public String keySelector() {
    return keySelector;
  }

  /**
   * Whether the key selector is applied in non-matching mode, so that the wallet claims key
   * identifiers that do not match {@link #keySelector()}.
   *
   * @return {@code true} if the selector match is inverted
   */
  @JsonProperty("keySelectorMustNotMatch")
  public boolean keySelectorMustNotMatch() {
    return keySelectorMustNotMatch;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof WalletInfo other
        && keySelectorMustNotMatch == other.keySelectorMustNotMatch
        && Objects.equals(name, other.name)
        && Objects.equals(keySelector, other.keySelector);
  }

  @Override
  public int hashCode() {
    return Objects.hash(name, keySelector, keySelectorMustNotMatch);
  }

  @Override
  public String toString() {
    return "WalletInfo{name=" + name + ", keySelector=" + keySelector + "}";
  }
}
