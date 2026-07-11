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

package org.lfdt.paladin.sdk.core.abi;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class AbiParameterTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Test
    void serializesNameAndTypeAlwaysAndOmitsEmptyOptionals() throws Exception {
        // omitempty optionals (internalType, components, indexed) are dropped; name/type always present.
        assertEquals("{\"name\":\"amount\",\"type\":\"uint256\"}",
                MAPPER.writeValueAsString(AbiParameter.of("amount", "uint256")));
    }

    @Test
    void serializesIndexedAndInternalTypeWhenSet() throws Exception {
        final AbiParameter p = AbiParameter.builder("from", "address").internalType("address").indexed(true).build();
        assertEquals("{\"name\":\"from\",\"type\":\"address\",\"internalType\":\"address\",\"indexed\":true}",
                MAPPER.writeValueAsString(p));
    }

    @Test
    void roundTripsTupleWithComponents() throws Exception {
        final AbiParameter tuple = AbiParameter.builder("order", "tuple")
                .component(AbiParameter.of("recipient", "address"))
                .component(AbiParameter.of("amount", "uint256"))
                .build();
        final AbiParameter parsed = MAPPER.readValue(MAPPER.writeValueAsString(tuple), AbiParameter.class);
        assertEquals(tuple, parsed);
        assertEquals(2, parsed.components().size());
        assertEquals("recipient", parsed.components().get(0).name());
    }

    @Test
    void deserializesFromSolidityStyleJson() throws Exception {
        final AbiParameter p = MAPPER.readValue(
                "{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}", AbiParameter.class);
        assertEquals("amount", p.name());
        assertEquals("uint256", p.type());
        assertEquals("uint256", p.internalType());
        assertFalse(p.indexed());
        assertTrue(p.components().isEmpty());
    }

    @Test
    void unnamedParameterDefaultsToEmptyName() throws Exception {
        final AbiParameter p = MAPPER.readValue("{\"type\":\"bool\"}", AbiParameter.class);
        assertEquals("", p.name());
        assertEquals("bool", p.type());
    }
}
