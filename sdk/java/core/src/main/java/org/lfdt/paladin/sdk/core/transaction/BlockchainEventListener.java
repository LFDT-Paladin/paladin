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

package org.lfdt.paladin.sdk.core.transaction;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import org.lfdt.paladin.sdk.core.types.Timestamp;

// Immutable; mirrors pldapi.BlockchainEventListener (a named stream of matched blockchain events).
// Used both as the input to ptx_createBlockchainEventListener and as the result of
// ptx_getBlockchainEventListener / ptx_queryBlockchainEventListeners; build one with the fluent
// builder to create a listener (created is server-assigned).
@JsonPropertyOrder({"name", "created", "started", "sources", "options"})
public final class BlockchainEventListener {

  private final String name;
  private final Timestamp created;
  private final Boolean started;
  private final List<BlockchainEventListenerSource> sources;
  private final BlockchainEventListenerOptions options;

  @JsonCreator
  BlockchainEventListener(
      @JsonProperty("name") String name,
      @JsonProperty("created") Timestamp created,
      @JsonProperty("started") Boolean started,
      @JsonProperty("sources") List<BlockchainEventListenerSource> sources,
      @JsonProperty("options") BlockchainEventListenerOptions options) {
    this.name = name;
    // A zero timestamp is "unset" (Go omitempty); normalize to null to keep round-trips clean.
    this.created = (created == null || created.isZero()) ? null : created;
    this.started = started;
    this.sources = sources == null ? Collections.emptyList() : List.copyOf(sources);
    this.options = options;
  }

  @JsonProperty("name")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String name() {
    return name;
  }

  @JsonProperty("created")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp created() {
    return created;
  }

  @JsonProperty("started")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Boolean started() {
    return started;
  }

  @JsonProperty("sources")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<BlockchainEventListenerSource> sources() {
    return sources;
  }

  @JsonProperty("options")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public BlockchainEventListenerOptions options() {
    return options;
  }

  public static Builder builder() {
    return new Builder();
  }

  @Override
  public String toString() {
    return "BlockchainEventListener{name=" + name + ", started=" + started + ", sources="
        + sources.size() + "}";
  }

  /** Fluent builder for {@link BlockchainEventListener}. */
  public static final class Builder {
    private String name;
    private Boolean started;
    private final List<BlockchainEventListenerSource> sources = new ArrayList<>();
    private BlockchainEventListenerOptions options;

    private Builder() {}

    public Builder name(String name) {
      this.name = name;
      return this;
    }

    public Builder started(Boolean started) {
      this.started = started;
      return this;
    }

    /** Adds a source to match events against. */
    public Builder source(BlockchainEventListenerSource source) {
      this.sources.add(source);
      return this;
    }

    /** Adds sources to match events against. */
    public Builder sources(List<BlockchainEventListenerSource> sources) {
      this.sources.addAll(sources);
      return this;
    }

    public Builder options(BlockchainEventListenerOptions options) {
      this.options = options;
      return this;
    }

    public BlockchainEventListener build() {
      return new BlockchainEventListener(name, null, started, sources, options);
    }
  }
}
