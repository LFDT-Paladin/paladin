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

/** Algorithm and key-type names used when registering identities for Zeto transactions. */
public final class ZetoVerifiers {

  /** Key-type label for an Idem3 Baby Jubjub compressed public key ({@code 0x}-prefixed hex). */
  public static final String IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X =
      "iden3_pubkey_babyjubjub_compressed_0x";

  private ZetoVerifiers() {}

  /**
   * The verifier algorithm string for Baby Jubjub SNARK proofs on the named Zeto domain.
   *
   * @param domainName the domain registration name; must not be {@code null}
   * @return the algorithm identifier understood by the node
   */
  public static String algorithmZetoSnarkBabyJubjub(final String domainName) {
    return "domain:" + Objects.requireNonNull(domainName, "domainName") + ":snark:babyjubjub";
  }
}
