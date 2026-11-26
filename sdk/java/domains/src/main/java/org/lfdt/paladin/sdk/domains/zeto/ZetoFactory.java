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
package org.lfdt.paladin.sdk.domains.zeto;

import java.util.Objects;
import java.util.Optional;
import org.lfdt.paladin.sdk.client.PaladinClient;

/**
 * Deploys Zeto tokens through a Zeto domain registered on the node.
 *
 * <pre>{@code
 * ZetoInstance token =
 *     new ZetoFactory(paladin, "zeto")
 *         .newZeto("alice", new ZetoConstructorParams("Zeto_Anon"), Optional.empty())
 *         .waitForDeploy()
 *         .join();
 * }</pre>
 */
public final class ZetoFactory {

  private final PaladinClient paladin;
  private final String domain;

  /**
   * Creates a factory for the named Zeto domain.
   *
   * @param paladin the client used to submit deployments; must not be {@code null}
   * @param domain the name the Zeto domain is registered under on the node; must not be {@code
   *     null}
   */
  public ZetoFactory(final PaladinClient paladin, final String domain) {
    this.paladin = Objects.requireNonNull(paladin, "paladin");
    this.domain = Objects.requireNonNull(domain, "domain");
  }

  /**
   * The name of the Zeto domain this factory deploys through.
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
  public ZetoFactory using(final PaladinClient paladin) {
    return new ZetoFactory(paladin, domain);
  }

  /**
   * Deploys a new Zeto token with an idempotency key, so that a retried submission is not deployed
   * twice.
   *
   * @param from lookup string of the identity that signs the deployment
   * @param params the token configuration
   * @param idempotencyKey a key unique to this deployment, or {@link Optional#empty()}
   * @return a handle on the in-flight deployment
   */
  public ZetoDeployment newZeto(
      final String from,
      final ZetoConstructorParams params,
      final Optional<String> idempotencyKey) {
    Objects.requireNonNull(params, "params");
    return new ZetoDeployment(
        paladin,
        domain,
        paladin
            .newTx()
            .privateTx()
            .domain(domain)
            .constructor()
            .abiEntry(ZetoAbi.CONSTRUCTOR)
            .from(from)
            .idempotencyKey(Objects.requireNonNull(idempotencyKey, "idempotencyKey").orElse(null))
            .inputs(params)
            .send());
  }
}
