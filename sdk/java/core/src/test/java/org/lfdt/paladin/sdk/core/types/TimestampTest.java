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
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.time.Instant;
import org.junit.jupiter.api.Test;

class TimestampTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();
  private static final long SECONDS = 1_700_000_000L;
  private static final long NANOS = SECONDS * 1_000_000_000L;

  @Test
  void infersResolutionFromMagnitude() {
    assertEquals(NANOS, Timestamp.fromUnix(SECONDS).unixNano());
    assertEquals(NANOS, Timestamp.fromUnix(SECONDS * 1_000L).unixNano());
    assertEquals(NANOS, Timestamp.fromUnix(NANOS).unixNano());
  }

  @Test
  void serializesAsRfc3339WithNanoPrecision() throws Exception {
    final Timestamp ts = Timestamp.ofInstant(Instant.ofEpochSecond(SECONDS, 123_456_789L));
    assertEquals("\"2023-11-14T22:13:20.123456789Z\"", MAPPER.writeValueAsString(ts));
  }

  @Test
  void roundTripsThroughJson() throws Exception {
    final Timestamp original = Timestamp.ofInstant(Instant.ofEpochSecond(SECONDS, 987_654_321L));
    assertEquals(original, MAPPER.readValue(MAPPER.writeValueAsString(original), Timestamp.class));
  }

  @Test
  void zeroSerializesToNullAndNullParsesToZero() throws Exception {
    assertEquals("null", MAPPER.writeValueAsString(Timestamp.ZERO));
    final Timestamp parsed = MAPPER.readValue("null", Timestamp.class);
    assertTrue(parsed.isZero());
    assertEquals(Timestamp.ZERO, parsed);
  }

  @Test
  void parsesRfc3339AndNumericStrings() {
    assertEquals(NANOS, Timestamp.fromString("2023-11-14T22:13:20Z").unixNano());
    assertEquals(NANOS, Timestamp.fromString("1700000000").unixNano());
  }

  @Test
  void deserializesFromJsonNumber() throws Exception {
    assertEquals(
        Timestamp.fromUnix(SECONDS), MAPPER.readValue(String.valueOf(SECONDS), Timestamp.class));
  }
}
