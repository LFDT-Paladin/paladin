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
import java.util.ArrayList;
import java.util.List;
import java.util.Objects;

/**
 * A single entry in a contract ABI — a function, constructor, event, error, or fallback/receive —
 * mirroring {@code abi.Entry} from firefly-signer.
 *
 * <p>Immutable and self-serializing. The {@code inputs} and {@code outputs} arrays are always
 * emitted (as {@code []} when empty); the remaining fields follow the Go {@code omitempty}
 * convention and are omitted when null/empty/false. Round-trips through any {@code ObjectMapper}.
 *
 * <p>The legacy {@code payable}/{@code constant} flags are superseded by {@link #stateMutability()}
 * but retained for compatibility with older ABIs.
 */
@JsonPropertyOrder({
  "type",
  "name",
  "payable",
  "constant",
  "anonymous",
  "stateMutability",
  "inputs",
  "outputs"
})
public final class AbiEntry {

  private final EntryType type;
  private final String name;
  private final boolean payable;
  private final boolean constant;
  private final boolean anonymous;
  private final StateMutability stateMutability;
  private final List<AbiParameter> inputs;
  private final List<AbiParameter> outputs;

  @JsonCreator
  AbiEntry(
      @JsonProperty("type") final EntryType type,
      @JsonProperty("name") final String name,
      @JsonProperty("payable") final boolean payable,
      @JsonProperty("constant") final boolean constant,
      @JsonProperty("anonymous") final boolean anonymous,
      @JsonProperty("stateMutability") final StateMutability stateMutability,
      @JsonProperty("inputs") final List<AbiParameter> inputs,
      @JsonProperty("outputs") final List<AbiParameter> outputs) {
    this.type = type;
    this.name = name == null ? "" : name;
    this.payable = payable;
    this.constant = constant;
    this.anonymous = anonymous;
    this.stateMutability = stateMutability;
    this.inputs = inputs == null ? List.of() : List.copyOf(inputs);
    this.outputs = outputs == null ? List.of() : List.copyOf(outputs);
  }

  /**
   * The kind of entry (function/event/error/...), or {@code null} if unspecified.
   *
   * @return the entry type, or {@code null} if unspecified
   */
  @JsonProperty("type")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EntryType type() {
    return type;
  }

  /**
   * The function/event/error name; empty for the constructor and fallback/receive entries.
   *
   * @return the entry name, or an empty string when it has none
   */
  @JsonProperty("name")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String name() {
    return name;
  }

  /**
   * Functions only (legacy): superseded by {@code stateMutability} {@code payable}/{@code
   * nonpayable}.
   *
   * @return the legacy {@code payable} flag
   */
  @JsonProperty("payable")
  @JsonInclude(JsonInclude.Include.NON_DEFAULT)
  public boolean payable() {
    return payable;
  }

  /**
   * Functions only (legacy): superseded by {@code stateMutability} {@code pure}/{@code view}.
   *
   * @return the legacy {@code constant} flag
   */
  @JsonProperty("constant")
  @JsonInclude(JsonInclude.Include.NON_DEFAULT)
  public boolean constant() {
    return constant;
  }

  /**
   * Events only: the event is emitted without a signature (topic[0] is not generated).
   *
   * @return {@code true} if the event is anonymous
   */
  @JsonProperty("anonymous")
  @JsonInclude(JsonInclude.Include.NON_DEFAULT)
  public boolean anonymous() {
    return anonymous;
  }

  /**
   * How the function interacts with blockchain state, or {@code null} if unspecified.
   *
   * @return the state mutability, or {@code null} if unspecified
   */
  @JsonProperty("stateMutability")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public StateMutability stateMutability() {
    return stateMutability;
  }

  /**
   * Input parameters of a function, or the fields of an event/error. Never null.
   *
   * @return the input parameters (never null, empty when there are none)
   */
  @JsonProperty("inputs")
  public List<AbiParameter> inputs() {
    return inputs;
  }

  /**
   * Functions only: the return values. Never null.
   *
   * @return the output parameters (never null, empty when there are none)
   */
  @JsonProperty("outputs")
  public List<AbiParameter> outputs() {
    return outputs;
  }

  /**
   * Starts a builder for an entry of the given type.
   *
   * @param type the kind of entry to build
   * @return a new builder
   */
  public static Builder builder(final EntryType type) {
    return new Builder(type);
  }

  /**
   * Starts a builder for a {@link EntryType#FUNCTION} with the given name.
   *
   * @param name the function name
   * @return a new builder
   */
  public static Builder function(final String name) {
    return new Builder(EntryType.FUNCTION).name(name);
  }

  /**
   * Starts a builder for an {@link EntryType#EVENT} with the given name.
   *
   * @param name the event name
   * @return a new builder
   */
  public static Builder event(final String name) {
    return new Builder(EntryType.EVENT).name(name);
  }

  /**
   * Starts a builder for an {@link EntryType#ERROR} with the given name.
   *
   * @param name the error name
   * @return a new builder
   */
  public static Builder error(final String name) {
    return new Builder(EntryType.ERROR).name(name);
  }

  /**
   * Starts a builder for an {@link EntryType#CONSTRUCTOR}.
   *
   * @return a new builder
   */
  public static Builder constructor() {
    return new Builder(EntryType.CONSTRUCTOR);
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof AbiEntry other
        && payable == other.payable
        && constant == other.constant
        && anonymous == other.anonymous
        && type == other.type
        && name.equals(other.name)
        && stateMutability == other.stateMutability
        && inputs.equals(other.inputs)
        && outputs.equals(other.outputs);
  }

  @Override
  public int hashCode() {
    return Objects.hash(type, name, payable, constant, anonymous, stateMutability, inputs, outputs);
  }

  @Override
  public String toString() {
    return "AbiEntry{type="
        + type
        + ", name="
        + name
        + ", inputs="
        + inputs.size()
        + ", outputs="
        + outputs.size()
        + "}";
  }

  /** Fluent builder for {@link AbiEntry}. */
  public static final class Builder {
    private final EntryType type;
    private String name;
    private boolean payable;
    private boolean constant;
    private boolean anonymous;
    private StateMutability stateMutability;
    private final List<AbiParameter> inputs = new ArrayList<>();
    private final List<AbiParameter> outputs = new ArrayList<>();

    private Builder(final EntryType type) {
      this.type = type;
    }

    /**
     * Sets the entry name.
     *
     * @param name the function/event/error name
     * @return this builder
     */
    public Builder name(final String name) {
      this.name = name;
      return this;
    }

    /**
     * Sets the legacy {@code payable} flag.
     *
     * @param payable the legacy {@code payable} flag
     * @return this builder
     */
    public Builder payable(final boolean payable) {
      this.payable = payable;
      return this;
    }

    /**
     * Sets the legacy {@code constant} flag.
     *
     * @param constant the legacy {@code constant} flag
     * @return this builder
     */
    public Builder constant(final boolean constant) {
      this.constant = constant;
      return this;
    }

    /**
     * Sets the {@code anonymous} flag (events only).
     *
     * @param anonymous whether the event is anonymous
     * @return this builder
     */
    public Builder anonymous(final boolean anonymous) {
      this.anonymous = anonymous;
      return this;
    }

    /**
     * Sets the state mutability.
     *
     * @param stateMutability how the function interacts with blockchain state
     * @return this builder
     */
    public Builder stateMutability(final StateMutability stateMutability) {
      this.stateMutability = stateMutability;
      return this;
    }

    /**
     * Adds a single input parameter.
     *
     * @param input the input parameter to add
     * @return this builder
     */
    public Builder input(final AbiParameter input) {
      this.inputs.add(input);
      return this;
    }

    /**
     * Adds several input parameters.
     *
     * @param inputs the input parameters to add
     * @return this builder
     */
    public Builder inputs(final List<AbiParameter> inputs) {
      this.inputs.addAll(inputs);
      return this;
    }

    /**
     * Adds a single output parameter.
     *
     * @param output the output parameter to add
     * @return this builder
     */
    public Builder output(final AbiParameter output) {
      this.outputs.add(output);
      return this;
    }

    /**
     * Adds several output parameters.
     *
     * @param outputs the output parameters to add
     * @return this builder
     */
    public Builder outputs(final List<AbiParameter> outputs) {
      this.outputs.addAll(outputs);
      return this;
    }

    /**
     * Builds the immutable {@link AbiEntry}.
     *
     * @return a new {@link AbiEntry} with the configured values
     */
    public AbiEntry build() {
      return new AbiEntry(
          type, name, payable, constant, anonymous, stateMutability, inputs, outputs);
    }
  }
}
