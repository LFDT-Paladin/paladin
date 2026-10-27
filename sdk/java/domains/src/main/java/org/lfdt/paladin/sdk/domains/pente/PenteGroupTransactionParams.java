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
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.types.EthAddress;

/**
 * Inputs to {@link PentePrivacyGroup#sendTransaction} and {@link PentePrivacyGroup#call}: one
 * function invocation on a private contract inside a group.
 *
 * @param from lookup string of the signing identity; optional for calls
 * @param to the address of the private contract
 * @param methodAbi the ABI entry of the function to invoke
 * @param data the function arguments, as a {@code Map}, a {@code List} of positional values or any
 *     value the SDK mapper can serialize; optional
 */
public record PenteGroupTransactionParams(
    String from, EthAddress to, AbiEntry methodAbi, Object data) {

  /**
   * Creates the parameters.
   *
   * @param from lookup string of the signing identity; optional for calls
   * @param to the address of the private contract; must not be {@code null}
   * @param methodAbi the ABI entry of the function to invoke; must not be {@code null}
   * @param data the function arguments; optional
   */
  public PenteGroupTransactionParams {
    Objects.requireNonNull(to, "to");
    Objects.requireNonNull(methodAbi, "methodAbi");
  }
}
