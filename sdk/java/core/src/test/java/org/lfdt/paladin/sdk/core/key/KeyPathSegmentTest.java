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
package org.lfdt.paladin.sdk.core.key;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class KeyPathSegmentTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTrips() throws Exception {
    KeyPathSegment segment = new KeyPathSegment("alice", 3);
    String json = MAPPER.writeValueAsString(segment);
    assertEquals("{\"name\":\"alice\",\"index\":3}", json);

    KeyPathSegment parsed = MAPPER.readValue(json, KeyPathSegment.class);
    assertEquals(segment, parsed);
    assertEquals("alice", parsed.name());
    assertEquals(3, parsed.index());
  }

  @Test
  void equalsAndHashCode() {
    KeyPathSegment a = new KeyPathSegment("alice", 3);
    KeyPathSegment b = new KeyPathSegment("alice", 3);
    KeyPathSegment differentIndex = new KeyPathSegment("alice", 4);
    KeyPathSegment differentName = new KeyPathSegment("bob", 3);

    assertEquals(a, a);
    assertEquals(a, b);
    assertEquals(a.hashCode(), b.hashCode());
    assertNotEquals(a, differentIndex);
    assertNotEquals(a, differentName);
    assertNotEquals(a, "not a segment");
    assertTrue(a.toString().contains("alice"));
  }
}
