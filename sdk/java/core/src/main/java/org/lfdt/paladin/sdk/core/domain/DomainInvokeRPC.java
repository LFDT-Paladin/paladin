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
package org.lfdt.paladin.sdk.core.domain;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.Objects;

/** A domain-specific RPC request evaluated in a privacy-group or smart-contract context. */
@JsonPropertyOrder({"method", "params"})
public final class DomainInvokeRPC {

  private final String method;
  private final JsonNode params;

  @JsonCreator
  DomainInvokeRPC(
      @JsonProperty("method") final String method, @JsonProperty("params") final JsonNode params) {
    this.method = method;
    this.params = params;
  }

  /**
   * The domain RPC method to invoke.
   *
   * @return the method name, or an empty string when unset
   */
  @JsonProperty("method")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String method() {
    return method;
  }

  /**
   * The parameters supplied to the domain RPC method.
   *
   * @return the method parameters, or {@code null} when unset
   */
  @JsonProperty("params")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode params() {
    return params;
  }

  /**
   * Starts a builder for the given domain RPC method.
   *
   * @param method the domain RPC method to invoke
   * @return a new builder
   */
  public static Builder builder(final String method) {
    return new Builder(method);
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof DomainInvokeRPC other
        && Objects.equals(method, other.method)
        && Objects.equals(params, other.params);
  }

  @Override
  public int hashCode() {
    return Objects.hash(method, params);
  }

  @Override
  public String toString() {
    return "DomainInvokeRPC{method=" + method + "}";
  }

  /** Fluent builder for {@link DomainInvokeRPC}. */
  public static final class Builder {
    private final String method;
    private JsonNode params;

    private Builder(final String method) {
      this.method = method;
    }

    /**
     * Sets the parameters supplied to the domain RPC method.
     *
     * @param params the method parameters
     * @return this builder
     */
    public Builder params(final JsonNode params) {
      this.params = params;
      return this;
    }

    /**
     * Builds the immutable {@link DomainInvokeRPC}.
     *
     * @return a new request with the configured values
     */
    public DomainInvokeRPC build() {
      return new DomainInvokeRPC(method, params);
    }
  }
}
