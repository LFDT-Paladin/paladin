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

// Immutable; mirrors pldapi.KeyVerifier. A resolved verifier (e.g. an address) for a key, together
// with the algorithm and verifier type it was produced under.
@JsonPropertyOrder({"verifier", "type", "algorithm"})
public final class KeyVerifier {

  private final String verifier;
  private final String type;
  private final String algorithm;

  @JsonCreator
  KeyVerifier(
      @JsonProperty("verifier") String verifier,
      @JsonProperty("type") String type,
      @JsonProperty("algorithm") String algorithm) {
    this.verifier = verifier;
    this.type = type;
    this.algorithm = algorithm;
  }

  @JsonProperty("verifier")
  public String verifier() {
    return verifier;
  }

  @JsonProperty("type")
  public String type() {
    return type;
  }

  @JsonProperty("algorithm")
  public String algorithm() {
    return algorithm;
  }

  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof KeyVerifier other
        && Objects.equals(verifier, other.verifier)
        && Objects.equals(type, other.type)
        && Objects.equals(algorithm, other.algorithm);
  }

  @Override
  public int hashCode() {
    return Objects.hash(verifier, type, algorithm);
  }

  @Override
  public String toString() {
    return "KeyVerifier{verifier=" + verifier + ", type=" + type + ", algorithm=" + algorithm + "}";
  }
}
