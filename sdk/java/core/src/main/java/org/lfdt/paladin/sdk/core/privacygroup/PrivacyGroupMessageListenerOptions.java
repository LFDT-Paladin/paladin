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
package org.lfdt.paladin.sdk.core.privacygroup;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;

/**
 * The delivery options for a privacy group message listener. Immutable; build one with the
 * {@linkplain #builder() fluent builder}.
 */
@JsonPropertyOrder({"excludeLocal"})
public final class PrivacyGroupMessageListenerOptions {

  private final boolean excludeLocal;

  @JsonCreator
  PrivacyGroupMessageListenerOptions(@JsonProperty("excludeLocal") final boolean excludeLocal) {
    this.excludeLocal = excludeLocal;
  }

  /**
   * Whether messages sent by this node are excluded from the stream.
   *
   * @return {@code true} if locally sent messages are excluded
   */
  @JsonProperty("excludeLocal")
  public boolean excludeLocal() {
    return excludeLocal;
  }

  /**
   * Starts an empty builder.
   *
   * @return a new builder
   */
  public static Builder builder() {
    return new Builder();
  }

  @Override
  public String toString() {
    return "PrivacyGroupMessageListenerOptions{excludeLocal=" + excludeLocal + "}";
  }

  /** Fluent builder for {@link PrivacyGroupMessageListenerOptions}. */
  public static final class Builder {
    private boolean excludeLocal;

    private Builder() {}

    /**
     * Sets whether messages sent by this node are excluded from the stream.
     *
     * @param excludeLocal whether to exclude locally sent messages
     * @return this builder
     */
    public Builder excludeLocal(final boolean excludeLocal) {
      this.excludeLocal = excludeLocal;
      return this;
    }

    /**
     * Builds the immutable {@link PrivacyGroupMessageListenerOptions}.
     *
     * @return a new {@link PrivacyGroupMessageListenerOptions} with the configured values
     */
    public PrivacyGroupMessageListenerOptions build() {
      return new PrivacyGroupMessageListenerOptions(excludeLocal);
    }
  }
}
