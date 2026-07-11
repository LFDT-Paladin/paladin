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

package org.lfdt.paladin.sdk.core.types;

import static org.junit.jupiter.api.Assertions.assertEquals;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.math.BigInteger;
import org.junit.jupiter.api.Test;

class HexInt256Test {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Test
    void formatsSignedHex() {
        assertEquals("0xff", HexInt256.of(255).to0xHex());
        assertEquals("-0x1", HexInt256.of(-1).to0xHex());
        assertEquals("0x0", HexInt256.of(0).to0xHex());
    }

    @Test
    void serializesNegativeAsString() throws Exception {
        assertEquals("\"-0xff\"", MAPPER.writeValueAsString(HexInt256.of(-255)));
    }

    @Test
    void deserializesNegativeFromString() throws Exception {
        assertEquals(HexInt256.of(-255), MAPPER.readValue("\"-0xff\"", HexInt256.class));
        assertEquals(HexInt256.of(-255), MAPPER.readValue("\"-255\"", HexInt256.class));
    }

    @Test
    void deserializesFromJsonNumber() throws Exception {
        assertEquals(HexInt256.of(-42), MAPPER.readValue("-42", HexInt256.class));
    }

    @Test
    void roundTripsNegativeThroughJson() throws Exception {
        final HexInt256 original = HexInt256.of(BigInteger.TWO.pow(200).negate());
        assertEquals(original, MAPPER.readValue(MAPPER.writeValueAsString(original), HexInt256.class));
    }
}
