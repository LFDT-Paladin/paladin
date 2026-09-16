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
package org.lfdt.paladin.sdk.core.domain;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class DomainInvokeRPCTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsMethodAndParams() throws Exception {
    final DomainInvokeRPC request =
        DomainInvokeRPC.builder("pente_getCodeHash")
            .params(MAPPER.readTree("[\"0x1234\"]"))
            .build();

    final String json = MAPPER.writeValueAsString(request);
    final DomainInvokeRPC parsed = MAPPER.readValue(json, DomainInvokeRPC.class);

    assertEquals("{\"method\":\"pente_getCodeHash\",\"params\":[\"0x1234\"]}", json);
    assertEquals("pente_getCodeHash", parsed.method());
    assertEquals("0x1234", parsed.params().get(0).asText());
    assertTrue(request.toString().contains("pente_getCodeHash"));
  }

  @Test
  void omitsUnsetValues() throws Exception {
    final DomainInvokeRPC request = DomainInvokeRPC.builder(null).build();

    assertEquals("{}", MAPPER.writeValueAsString(request));
    assertNull(request.method());
    assertNull(request.params());
  }

  @Test
  void implementsValueEquality() throws Exception {
    final DomainInvokeRPC request =
        DomainInvokeRPC.builder("pente_getCodeHash")
            .params(MAPPER.readTree("[\"0x1234\"]"))
            .build();
    final DomainInvokeRPC equal =
        DomainInvokeRPC.builder("pente_getCodeHash")
            .params(MAPPER.readTree("[\"0x1234\"]"))
            .build();
    final DomainInvokeRPC unequal = DomainInvokeRPC.builder("pente_getBalance").build();

    assertEquals(request, request);
    assertEquals(request, equal);
    assertEquals(request.hashCode(), equal.hashCode());
    assertNotEquals(request, unequal);
    assertNotEquals(request, null);
    assertNotEquals(request, "different type");
  }
}
