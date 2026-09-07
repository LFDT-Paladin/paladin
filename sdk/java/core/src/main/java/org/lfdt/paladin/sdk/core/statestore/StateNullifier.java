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
package org.lfdt.paladin.sdk.core.statestore;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * A nullifier a domain uses to spend a state without revealing the state id. A spend record is
 * written against the nullifier rather than the state. Immutable.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
@JsonPropertyOrder({"id", "spent"})
public final class StateNullifier {

  private final HexBytes id;
  private final StateSpendRecord spent;

  @JsonCreator
  StateNullifier(
      @JsonProperty("id") final HexBytes id, @JsonProperty("spent") final StateSpendRecord spent) {
    this.id = id;
    this.spent = spent;
  }

  /**
   * The nullifier identifier.
   *
   * @return the nullifier id
   */
  @JsonProperty("id")
  public HexBytes id() {
    return id;
  }

  /**
   * The spend record written against this nullifier, or {@code null} if it has not been spent.
   *
   * @return the spend record, or {@code null}
   */
  @JsonProperty("spent")
  public StateSpendRecord spent() {
    return spent;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof StateNullifier other
        && Objects.equals(id, other.id)
        && Objects.equals(spent, other.spent);
  }

  @Override
  public int hashCode() {
    return Objects.hash(id, spent);
  }

  @Override
  public String toString() {
    return "StateNullifier{id=" + id + ", spent=" + spent + "}";
  }
}
