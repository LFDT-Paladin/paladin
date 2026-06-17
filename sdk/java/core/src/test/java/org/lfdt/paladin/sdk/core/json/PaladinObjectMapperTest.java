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

package org.lfdt.paladin.sdk.core.json;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNotSame;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.math.BigInteger;
import java.time.Instant;
import java.util.Map;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.Timestamp;

class PaladinObjectMapperTest {

    private static final ObjectMapper MAPPER = PaladinObjectMapper.shared();

    @Test
    void sharedReturnsSameInstanceAndCreateReturnsFreshOnes() {
        assertSame(PaladinObjectMapper.shared(), PaladinObjectMapper.shared());
        assertNotSame(PaladinObjectMapper.create(), PaladinObjectMapper.create());
        assertNotSame(PaladinObjectMapper.shared(), PaladinObjectMapper.create());
    }

    @Test
    void ignoresUnknownProperties() throws Exception {
        // A field the client does not know about must not fail deserialization (forward compatibility).
        Holder h = MAPPER.readValue("{\"name\":\"noto\",\"futureServerField\":42}", Holder.class);
        assertEquals("noto", h.name);
    }

    @Test
    void omitsNullProperties() throws Exception {
        assertEquals("{}", MAPPER.writeValueAsString(new Holder()));
    }

    @Test
    void writesJavaTimeAsIso8601StringNotNumericTimestamp() throws Exception {
        Instant instant = Instant.ofEpochSecond(1_700_000_000L);
        String json = MAPPER.writeValueAsString(instant);
        assertEquals("\"2023-11-14T22:13:20Z\"", json);
        assertEquals(instant, MAPPER.readValue(json, Instant.class));
    }

    @Test
    void usesBigIntegerForUntypedIntegersToPreservePrecision() throws Exception {
        // 2^64 + 1 — would lose precision if bound through double.
        String big = "18446744073709551617";
        Map<String, Object> m = MAPPER.readValue("{\"v\":" + big + "}", new TypeReference<>() {});
        assertInstanceOf(BigInteger.class, m.get("v"));
        assertEquals(new BigInteger(big), m.get("v"));
    }

    @Test
    void roundTripsSdkPrimitiveTypes() throws Exception {
        HexBytes bytes = HexBytes.fromString("0xdeadbeef");
        assertEquals(bytes, MAPPER.readValue(MAPPER.writeValueAsString(bytes), HexBytes.class));

        EthAddress address = EthAddress.fromString("0x1f9090aaE28b8a3dCeaDf281B0F12828e676c326");
        assertEquals(address, MAPPER.readValue(MAPPER.writeValueAsString(address), EthAddress.class));

        HexUint256 amount = HexUint256.of(BigInteger.valueOf(1_000_000L));
        assertEquals(amount, MAPPER.readValue(MAPPER.writeValueAsString(amount), HexUint256.class));

        Timestamp ts = Timestamp.ofInstant(Instant.ofEpochSecond(1_700_000_000L, 123_456_789L));
        assertEquals(ts, MAPPER.readValue(MAPPER.writeValueAsString(ts), Timestamp.class));
    }

    @Test
    void zeroTimestampStillSerializesToNullUnderNonNullInclusion() throws Exception {
        // NON_NULL inclusion suppresses null *references*; a zero Timestamp is a non-null object
        // whose serializer writes JSON null, so the field is present rather than dropped.
        String json = MAPPER.writeValueAsString(new TimeHolder(Timestamp.ZERO));
        assertTrue(json.contains("\"at\":null"));
        TimeHolder parsed = MAPPER.readValue(json, TimeHolder.class);
        assertFalse(parsed.at == null);
        assertTrue(parsed.at.isZero());
    }

    static final class Holder {
        public String name;
        public String optional;
    }

    static final class TimeHolder {
        public Timestamp at;

        @SuppressWarnings("unused")
        TimeHolder() {
        }

        TimeHolder(Timestamp at) {
            this.at = at;
        }
    }
}
