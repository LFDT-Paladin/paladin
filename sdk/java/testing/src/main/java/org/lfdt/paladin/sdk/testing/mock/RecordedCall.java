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
package org.lfdt.paladin.sdk.testing.mock;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;

/**
 * One JSON-RPC call received by a {@link MockRpcClient}.
 *
 * @param method the JSON-RPC method name
 * @param params the positional parameters, as they would have been sent on the wire
 */
public record RecordedCall(String method, List<JsonNode> params) {

  /**
   * Creates a call record, copying the parameters.
   *
   * @param method the JSON-RPC method name
   * @param params the positional parameters as JSON
   */
  public RecordedCall {
    params = List.copyOf(params);
  }

  /**
   * Returns one positional parameter as JSON.
   *
   * @param index the zero-based position
   * @return the parameter
   * @throws IndexOutOfBoundsException if the call had fewer parameters
   */
  public JsonNode param(final int index) {
    return params.get(index);
  }

  /**
   * Returns one positional parameter converted to an SDK type.
   *
   * @param index the zero-based position
   * @param type the type to convert to, e.g. {@code TransactionInput.class}
   * @param <T> the parameter type
   * @return the converted parameter
   */
  public <T> T param(final int index, final Class<T> type) {
    return PaladinObjectMapper.shared().convertValue(params.get(index), type);
  }
}
