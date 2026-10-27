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
package org.lfdt.paladin.sdk.domains.pente;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupInput;

/**
 * Inputs to {@link PenteFactory#newPrivacyGroup}: the members and EVM configuration of a new
 * privacy group. Start from {@link #of(String...)} or {@link #of(List)}, then refine with the
 * {@code with*} methods. Unset configuration values fall back to the domain's defaults.
 *
 * @param name a human-readable group name; optional
 * @param members lookup strings of the identities in the group
 * @param evmVersion the EVM version the group executes with, such as {@code "shanghai"}; optional
 * @param endorsementType how transactions in the group are endorsed, such as {@code
 *     "group_scoped_identities"}; optional
 * @param externalCallsEnabled whether private contracts may trigger calls on the base ledger;
 *     optional
 * @param properties extra properties recorded with the group; defaults to empty
 */
public record PentePrivacyGroupParams(
    String name,
    List<String> members,
    String evmVersion,
    String endorsementType,
    Boolean externalCallsEnabled,
    Map<String, String> properties) {

  /**
   * Creates the parameters, copying the collections and defaulting unset properties to empty.
   *
   * @param name a human-readable group name; optional
   * @param members lookup strings of the identities in the group; must not be {@code null}
   * @param evmVersion the EVM version the group executes with; optional
   * @param endorsementType how transactions in the group are endorsed; optional
   * @param externalCallsEnabled whether private contracts may call the base ledger; optional
   * @param properties extra properties recorded with the group; defaults to empty
   */
  public PentePrivacyGroupParams {
    members = List.copyOf(Objects.requireNonNull(members, "members"));
    properties = properties == null ? Map.of() : Map.copyOf(properties);
  }

  /**
   * A group of the given members, with the domain's default configuration.
   *
   * @param members lookup strings of the identities in the group
   * @return the parameters
   */
  public static PentePrivacyGroupParams of(final String... members) {
    return of(List.of(members));
  }

  /**
   * A group of the given members, with the domain's default configuration.
   *
   * @param members lookup strings of the identities in the group
   * @return the parameters
   */
  public static PentePrivacyGroupParams of(final List<String> members) {
    return new PentePrivacyGroupParams(null, members, null, null, null, null);
  }

  /**
   * Copies these parameters with a new group name.
   *
   * @param name the group name
   * @return the updated parameters
   */
  public PentePrivacyGroupParams withName(final String name) {
    return new PentePrivacyGroupParams(
        name, members, evmVersion, endorsementType, externalCallsEnabled, properties);
  }

  /**
   * Copies these parameters with a new EVM version.
   *
   * @param evmVersion the EVM version, such as {@code "shanghai"}
   * @return the updated parameters
   */
  public PentePrivacyGroupParams withEvmVersion(final String evmVersion) {
    return new PentePrivacyGroupParams(
        name, members, evmVersion, endorsementType, externalCallsEnabled, properties);
  }

  /**
   * Copies these parameters with a new endorsement type.
   *
   * @param endorsementType the endorsement type, such as {@code "group_scoped_identities"}
   * @return the updated parameters
   */
  public PentePrivacyGroupParams withEndorsementType(final String endorsementType) {
    return new PentePrivacyGroupParams(
        name, members, evmVersion, endorsementType, externalCallsEnabled, properties);
  }

  /**
   * Copies these parameters, enabling or disabling calls from private contracts to the base ledger.
   *
   * @param externalCallsEnabled whether external calls are enabled
   * @return the updated parameters
   */
  public PentePrivacyGroupParams withExternalCallsEnabled(final boolean externalCallsEnabled) {
    return new PentePrivacyGroupParams(
        name, members, evmVersion, endorsementType, externalCallsEnabled, properties);
  }

  /**
   * Copies these parameters with an extra property recorded on the group.
   *
   * @param key the property name
   * @param value the property value
   * @return the updated parameters
   */
  public PentePrivacyGroupParams withProperty(final String key, final String value) {
    final Map<String, String> updated = new LinkedHashMap<>(properties);
    updated.put(key, value);
    return new PentePrivacyGroupParams(
        name, members, evmVersion, endorsementType, externalCallsEnabled, updated);
  }

  /** Builds the {@code pgroup_createGroup} request for the given domain. */
  PrivacyGroupInput toInput(final String domain) {
    final PrivacyGroupInput.Builder input =
        PrivacyGroupInput.builder(domain).name(name).members(members).properties(properties);
    if (evmVersion != null) {
      input.configuration("evmVersion", evmVersion);
    }
    if (endorsementType != null) {
      input.configuration("endorsementType", endorsementType);
    }
    if (externalCallsEnabled != null) {
      input.configuration("externalCallsEnabled", externalCallsEnabled.toString());
    }
    return input.build();
  }
}
