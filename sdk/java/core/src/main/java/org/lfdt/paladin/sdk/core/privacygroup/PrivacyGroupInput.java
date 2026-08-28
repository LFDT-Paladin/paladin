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
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * The specification for a new privacy group. Immutable; build one with the {@linkplain
 * #builder(String) fluent builder}.
 *
 * <p>{@link #domain()} and {@link #members()} are required by the node. {@link #properties()} are
 * free-form and recorded in the genesis state; {@link #configuration()} is interpreted by the
 * domain. Both maps are omitted from the JSON when empty.
 */
@JsonPropertyOrder({
  "domain",
  "members",
  "name",
  "properties",
  "configuration",
  "transactionOptions"
})
public final class PrivacyGroupInput {

  private final String domain;
  private final List<String> members;
  private final String name;
  private final Map<String, String> properties;
  private final Map<String, String> configuration;
  private final PrivacyGroupTXOptions transactionOptions;

  @JsonCreator
  PrivacyGroupInput(
      @JsonProperty("domain") final String domain,
      @JsonProperty("members") final List<String> members,
      @JsonProperty("name") final String name,
      @JsonProperty("properties") final Map<String, String> properties,
      @JsonProperty("configuration") final Map<String, String> configuration,
      @JsonProperty("transactionOptions") final PrivacyGroupTXOptions transactionOptions) {
    this.domain = domain;
    this.members = members == null ? List.of() : List.copyOf(members);
    this.name = name;
    this.properties = properties == null ? Map.of() : Map.copyOf(properties);
    this.configuration = configuration == null ? Map.of() : Map.copyOf(configuration);
    this.transactionOptions = transactionOptions;
  }

  /**
   * The domain the group is created in.
   *
   * @return the domain name, or an empty string when unset
   */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  /**
   * The identity locators of the group's members.
   *
   * @return the members, never {@code null} (empty when unset)
   */
  @JsonProperty("members")
  public List<String> members() {
    return members;
  }

  /**
   * The name of the group.
   *
   * @return the group name, or an empty string when unset
   */
  @JsonProperty("name")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String name() {
    return name;
  }

  /**
   * The free-form properties to record in the group's genesis state.
   *
   * @return the properties, never {@code null} (empty when unset)
   */
  @JsonProperty("properties")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public Map<String, String> properties() {
    return properties;
  }

  /**
   * The domain-interpreted configuration for the group.
   *
   * @return the configuration, never {@code null} (empty when unset)
   */
  @JsonProperty("configuration")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public Map<String, String> configuration() {
    return configuration;
  }

  /**
   * The submission options applied to the group's genesis transaction.
   *
   * @return the transaction options, or {@code null} to use the node's defaults
   */
  @JsonProperty("transactionOptions")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public PrivacyGroupTXOptions transactionOptions() {
    return transactionOptions;
  }

  /**
   * Starts a builder for a group in the given domain.
   *
   * @param domain the domain to create the group in
   * @return a new builder
   */
  public static Builder builder(final String domain) {
    return new Builder(domain);
  }

  @Override
  public String toString() {
    return "PrivacyGroupInput{domain=" + domain + ", name=" + name + ", members=" + members + "}";
  }

  /** Fluent builder for {@link PrivacyGroupInput}. */
  public static final class Builder {
    private final String domain;
    private final List<String> members = new ArrayList<>();
    private String name;
    private final Map<String, String> properties = new LinkedHashMap<>();
    private final Map<String, String> configuration = new LinkedHashMap<>();
    private PrivacyGroupTXOptions transactionOptions;

    private Builder(final String domain) {
      this.domain = domain;
    }

    /**
     * Adds a member to the group.
     *
     * @param member the identity locator of a member
     * @return this builder
     */
    public Builder member(final String member) {
      this.members.add(member);
      return this;
    }

    /**
     * Adds members to the group.
     *
     * @param members the identity locators of the members to add
     * @return this builder
     */
    public Builder members(final List<String> members) {
      this.members.addAll(members);
      return this;
    }

    /**
     * Sets the name of the group.
     *
     * @param name the group name
     * @return this builder
     */
    public Builder name(final String name) {
      this.name = name;
      return this;
    }

    /**
     * Adds a free-form property to record in the genesis state.
     *
     * @param key the property key
     * @param value the property value
     * @return this builder
     */
    public Builder property(final String key, final String value) {
      this.properties.put(key, value);
      return this;
    }

    /**
     * Adds free-form properties to record in the genesis state.
     *
     * @param properties the properties to add
     * @return this builder
     */
    public Builder properties(final Map<String, String> properties) {
      this.properties.putAll(properties);
      return this;
    }

    /**
     * Adds a domain-interpreted configuration entry.
     *
     * @param key the configuration key
     * @param value the configuration value
     * @return this builder
     */
    public Builder configuration(final String key, final String value) {
      this.configuration.put(key, value);
      return this;
    }

    /**
     * Adds domain-interpreted configuration entries.
     *
     * @param configuration the configuration entries to add
     * @return this builder
     */
    public Builder configuration(final Map<String, String> configuration) {
      this.configuration.putAll(configuration);
      return this;
    }

    /**
     * Sets the submission options for the group's genesis transaction.
     *
     * @param transactionOptions the transaction options
     * @return this builder
     */
    public Builder transactionOptions(final PrivacyGroupTXOptions transactionOptions) {
      this.transactionOptions = transactionOptions;
      return this;
    }

    /**
     * Builds the immutable {@link PrivacyGroupInput}.
     *
     * @return a new {@link PrivacyGroupInput} with the configured values
     */
    public PrivacyGroupInput build() {
      return new PrivacyGroupInput(
          domain, members, name, properties, configuration, transactionOptions);
    }
  }
}
