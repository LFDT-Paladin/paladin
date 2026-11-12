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
package org.lfdt.paladin.sdk.testing.containers;

import org.junit.jupiter.api.extension.ExtensionContext;
import org.junit.jupiter.api.extension.ParameterContext;
import org.junit.jupiter.api.extension.ParameterResolver;
import org.lfdt.paladin.sdk.client.PaladinClient;

/**
 * JUnit 5 extension that injects the {@link PaladinTestNetwork#shared() shared network} into test
 * constructors, lifecycle methods and test methods.
 *
 * <pre>{@code
 * @Tag("testcontainers")
 * @ExtendWith(PaladinNetworkExtension.class)
 * class MyNodeTest {
 *   @Test
 *   void readsWallets(PaladinClient client) {
 *     assertFalse(client.keyManager().wallets().join().isEmpty());
 *   }
 * }
 * }</pre>
 *
 * <p>Resolves three parameter types: {@link PaladinTestNetwork}, {@link PaladinNodeContainer} and
 * {@link PaladinClient}, the last two for {@value PaladinTestNetwork#DEFAULT_NODE}. Injected
 * clients belong to the network; tests must not close them.
 */
public final class PaladinNetworkExtension implements ParameterResolver {

  /** Creates the extension; JUnit instantiates it through {@code @ExtendWith}. */
  public PaladinNetworkExtension() {}

  @Override
  public boolean supportsParameter(
      final ParameterContext parameterContext, final ExtensionContext extensionContext) {
    final Class<?> type = parameterContext.getParameter().getType();
    return type == PaladinTestNetwork.class
        || type == PaladinNodeContainer.class
        || type == PaladinClient.class;
  }

  @Override
  public Object resolveParameter(
      final ParameterContext parameterContext, final ExtensionContext extensionContext) {
    final PaladinTestNetwork network = PaladinTestNetwork.shared();
    final Class<?> type = parameterContext.getParameter().getType();
    if (type == PaladinTestNetwork.class) {
      return network;
    }
    if (type == PaladinNodeContainer.class) {
      return network.node(PaladinTestNetwork.DEFAULT_NODE);
    }
    return network.client(PaladinTestNetwork.DEFAULT_NODE);
  }
}
