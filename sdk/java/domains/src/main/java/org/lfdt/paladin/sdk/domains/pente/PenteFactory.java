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

import java.util.Objects;
import java.util.Optional;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * Creates and resumes Pente privacy groups through a Pente domain registered on the node.
 *
 * <pre>{@code
 * PentePrivacyGroup group =
 *     new PenteFactory(paladin, "pente")
 *         .newPrivacyGroup(PentePrivacyGroupParams.of("alice@node1", "bob@node2"))
 *         .waitForDeploy()
 *         .join();
 * }</pre>
 */
public final class PenteFactory {

  private final PaladinClient paladin;
  private final String domain;

  /**
   * Creates a factory for the named Pente domain.
   *
   * @param paladin the client used to create groups; must not be {@code null}
   * @param domain the name the Pente domain is registered under on the node; must not be {@code
   *     null}
   */
  public PenteFactory(final PaladinClient paladin, final String domain) {
    this.paladin = Objects.requireNonNull(paladin, "paladin");
    this.domain = Objects.requireNonNull(domain, "domain");
  }

  /**
   * The name of the Pente domain this factory creates groups in.
   *
   * @return the domain name
   */
  public String domain() {
    return domain;
  }

  /**
   * Returns a factory for the same domain that submits through another client, for example a
   * different node.
   *
   * @param paladin the client to submit through
   * @return a new factory
   */
  public PenteFactory using(final PaladinClient paladin) {
    return new PenteFactory(paladin, domain);
  }

  /**
   * Creates a new privacy group. The node submits the group's genesis transaction; wait for it with
   * {@link PentePrivacyGroupDeployment#waitForDeploy()}.
   *
   * @param params the group members and configuration; must not be {@code null}
   * @return a handle on the in-flight group creation
   */
  public PentePrivacyGroupDeployment newPrivacyGroup(final PentePrivacyGroupParams params) {
    Objects.requireNonNull(params, "params");
    return new PentePrivacyGroupDeployment(
        paladin, paladin.privacyGroups().createGroup(params.toInput(domain)));
  }

  /**
   * Looks up an existing privacy group in this domain.
   *
   * @param id the group identifier
   * @return a future completing with the group, or empty if the node has no such group or its
   *     genesis transaction has not yet been mined
   */
  public CompletableFuture<Optional<PentePrivacyGroup>> resumePrivacyGroup(final HexBytes id) {
    return paladin
        .privacyGroups()
        .getGroupById(domain, id)
        .thenApply(
            group ->
                group == null || group.contractAddress() == null
                    ? Optional.empty()
                    : Optional.of(new PentePrivacyGroup(paladin, group)));
  }

  /**
   * Looks up an existing privacy group in this domain by its hex identifier.
   *
   * @param id the hex group identifier
   * @return a future completing with the group, or empty if the node has no such group or its
   *     genesis transaction has not yet been mined
   */
  public CompletableFuture<Optional<PentePrivacyGroup>> resumePrivacyGroup(final String id) {
    return resumePrivacyGroup(HexBytes.fromString(id));
  }
}
