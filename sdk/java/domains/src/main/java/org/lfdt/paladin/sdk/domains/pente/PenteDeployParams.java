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

import java.util.List;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * Inputs to {@link PentePrivacyGroup#deploy}: a private contract to deploy inside a group.
 *
 * @param abi the contract ABI; its constructor, if any, describes {@code inputs}. Defaults to empty
 * @param bytecode the compiled contract bytecode
 * @param from lookup string of the identity that signs the deployment
 * @param inputs the constructor arguments, as a {@code Map}, a {@code List} of positional values or
 *     any value the SDK mapper can serialize; optional
 */
public record PenteDeployParams(List<AbiEntry> abi, HexBytes bytecode, String from, Object inputs) {

  /**
   * Creates the parameters, copying the ABI and defaulting it to empty.
   *
   * @param abi the contract ABI; defaults to empty
   * @param bytecode the compiled contract bytecode; must not be {@code null}
   * @param from lookup string of the identity that signs the deployment
   * @param inputs the constructor arguments; optional
   */
  public PenteDeployParams {
    abi = abi == null ? List.of() : List.copyOf(abi);
    Objects.requireNonNull(bytecode, "bytecode");
  }
}
