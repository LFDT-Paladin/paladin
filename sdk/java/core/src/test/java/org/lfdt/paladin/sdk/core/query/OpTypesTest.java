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
package org.lfdt.paladin.sdk.core.query;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.TextNode;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.Test;

/** Value semantics of the three filter operand shapes: equality, hashing, and null handling. */
class OpTypesTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  private static JsonNode node(final String text) {
    return TextNode.valueOf(text);
  }

  @Test
  void opEqualityCoversEveryField() {
    final Op op = new Op("failureMessage", true, true);

    assertEquals(op, op);
    assertEquals(op, new Op("failureMessage", true, true));
    assertEquals(op.hashCode(), new Op("failureMessage", true, true).hashCode());

    assertNotEquals(op, new Op("other", true, true));
    assertNotEquals(op, new Op("failureMessage", false, true));
    assertNotEquals(op, new Op("failureMessage", true, false));
    assertNotEquals(op, null);
    assertNotEquals(op, "failureMessage");
  }

  @Test
  void opSingleValEqualityCoversEveryField() {
    final OpSingleVal op = new OpSingleVal("type", true, true, node("private"));

    assertEquals(op, op);
    assertEquals(op, new OpSingleVal("type", true, true, node("private")));
    assertEquals(op.hashCode(), new OpSingleVal("type", true, true, node("private")).hashCode());

    assertNotEquals(op, new OpSingleVal("other", true, true, node("private")));
    assertNotEquals(op, new OpSingleVal("type", false, true, node("private")));
    assertNotEquals(op, new OpSingleVal("type", true, false, node("private")));
    assertNotEquals(op, new OpSingleVal("type", true, true, node("public")));
    assertNotEquals(op, new OpSingleVal("type", true, true, null));
    assertNotEquals(op, null);
    assertNotEquals(op, new Op("type", true, true));
  }

  @Test
  void opMultiValEqualityCoversEveryField() {
    final List<JsonNode> values = List.of(node("a"), node("b"));
    final OpMultiVal op = new OpMultiVal("status", true, true, values);

    assertEquals(op, op);
    assertEquals(op, new OpMultiVal("status", true, true, values));
    assertEquals(op.hashCode(), new OpMultiVal("status", true, true, values).hashCode());

    assertNotEquals(op, new OpMultiVal("other", true, true, values));
    assertNotEquals(op, new OpMultiVal("status", false, true, values));
    assertNotEquals(op, new OpMultiVal("status", true, false, values));
    assertNotEquals(op, new OpMultiVal("status", true, true, List.of(node("a"))));
    assertNotEquals(op, null);
    assertNotEquals(op, new Op("status", true, true));
  }

  @Test
  void nullFieldDefaultsToEmptyStringOnEveryOperand() {
    assertEquals("", new Op(null, false, false).field());
    assertEquals("", new OpSingleVal(null, false, false, node("v")).field());
    assertEquals("", new OpMultiVal(null, false, false, List.of()).field());
  }

  @Test
  void nullValuesDefaultToAnEmptyList() {
    final OpMultiVal op = new OpMultiVal("status", false, false, null);
    assertTrue(op.values().isEmpty());
    assertNull(new OpSingleVal("type", false, false, null).value());
  }

  @Test
  void multiValCopiesTheCallersList() {
    final List<JsonNode> mutable = new ArrayList<>(List.of(node("a")));
    final OpMultiVal op = new OpMultiVal("status", false, false, mutable);
    mutable.add(node("b"));
    assertEquals(1, op.values().size());
    assertThrows(UnsupportedOperationException.class, () -> op.values().add(node("c")));
  }

  @Test
  void toStringSummarizesTheOperand() {
    assertEquals("Op{field=a}", new Op("a", false, false).toString());
    assertEquals("Op{field=a, not, caseInsensitive}", new Op("a", true, true).toString());
    assertEquals(
        "OpSingleVal{field=a, value=\"v\", not}",
        new OpSingleVal("a", true, false, node("v")).toString());
    assertEquals(
        "OpSingleVal{field=a, value=\"v\"}",
        new OpSingleVal("a", false, false, node("v")).toString());
    assertEquals(
        "OpMultiVal{field=a, values=1}",
        new OpMultiVal("a", false, false, List.of(node("v"))).toString());
    assertEquals(
        "OpMultiVal{field=a, values=0, not}",
        new OpMultiVal("a", true, false, List.of()).toString());
  }

  @Test
  void operandsRoundTripThroughJackson() throws Exception {
    final Op op = new Op("failureMessage", true, false);
    assertEquals(op, MAPPER.readValue(MAPPER.writeValueAsString(op), Op.class));

    final OpSingleVal single = new OpSingleVal("type", false, true, node("private"));
    assertEquals(single, MAPPER.readValue(MAPPER.writeValueAsString(single), OpSingleVal.class));

    final OpMultiVal multi = new OpMultiVal("status", true, false, List.of(node("a"), node("b")));
    assertEquals(multi, MAPPER.readValue(MAPPER.writeValueAsString(multi), OpMultiVal.class));
  }
}
