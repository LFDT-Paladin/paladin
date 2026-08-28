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
package org.lfdt.paladin.sdk.core.privacygroup;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.transaction.PublicTxOptions;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;
import org.lfdt.paladin.sdk.core.types.Timestamp;

class PrivacyGroupTypesTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  private static final String GROUP_ID = "0xfeed";
  private static final String ADDRESS = "0x" + "11".repeat(20);
  private static final String SALT = "0x" + "22".repeat(32);
  private static final String SCHEMA = "0x" + "33".repeat(32);

  // ---- PrivacyGroup -------------------------------------------------------

  @Test
  void privacyGroupRoundTripsFully() throws Exception {
    final UUID genesisTx = UUID.randomUUID();
    final String json =
        "{\"id\":\""
            + GROUP_ID
            + "\",\"domain\":\"pente\",\"created\":\"1700000000000000000\",\"name\":\"g1\","
            + "\"members\":[\"me@node1\",\"you@node2\"],"
            + "\"properties\":{\"k\":\"v\"},\"configuration\":{\"evmVersion\":\"shanghai\"},"
            + "\"genesisSalt\":\""
            + SALT
            + "\",\"genesisSchema\":\""
            + SCHEMA
            + "\",\"genesisTransaction\":\""
            + genesisTx
            + "\",\"contractAddress\":\""
            + ADDRESS
            + "\"}";

    final PrivacyGroup group = MAPPER.readValue(json, PrivacyGroup.class);

    assertEquals(HexBytes.fromString(GROUP_ID), group.id());
    assertEquals("pente", group.domain());
    assertEquals(Timestamp.ofUnixNano(1700000000000000000L), group.created());
    assertEquals("g1", group.name());
    assertEquals(List.of("me@node1", "you@node2"), group.members());
    assertEquals(Map.of("k", "v"), group.properties());
    assertEquals(Map.of("evmVersion", "shanghai"), group.configuration());
    assertEquals(Bytes32.fromString(SALT), group.genesisSalt());
    assertEquals(Bytes32.fromString(SCHEMA), group.genesisSchema());
    assertEquals(genesisTx, group.genesisTransaction());
    assertEquals(EthAddress.fromString(ADDRESS), group.contractAddress());
    assertTrue(group.toString().contains("domain=pente"));

    final PrivacyGroup reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(group), PrivacyGroup.class);
    assertEquals(group.id(), reparsed.id());
    assertEquals(group.members(), reparsed.members());
    assertEquals(group.contractAddress(), reparsed.contractAddress());
  }

  @Test
  void privacyGroupNormalizesUnsetCollectionsAndTimestamp() throws Exception {
    final PrivacyGroup group =
        MAPPER.readValue("{\"domain\":\"pente\",\"created\":\"0\"}", PrivacyGroup.class);

    assertNull(group.created());
    assertNull(group.id());
    assertNull(group.contractAddress());
    assertTrue(group.members().isEmpty());
    assertTrue(group.properties().isEmpty());
    assertTrue(group.configuration().isEmpty());

    // Null-valued fields drop out; the never-null collections still serialize.
    final String json = MAPPER.writeValueAsString(group);
    assertFalse(json.contains("\"created\""));
    assertFalse(json.contains("\"contractAddress\""));
    assertTrue(json.contains("\"members\":[]"));
  }

  // ---- PrivacyGroupInput / PrivacyGroupTXOptions --------------------------

  @Test
  void privacyGroupInputBuilderRoundTrips() throws Exception {
    final PrivacyGroupInput input =
        PrivacyGroupInput.builder("pente")
            .name("g1")
            .member("me@node1")
            .members(List.of("you@node2"))
            .property("k", "v")
            .properties(Map.of("k2", "v2"))
            .configuration("evmVersion", "shanghai")
            .configuration(Map.of("endorsementType", "group_scoped_identities"))
            .transactionOptions(
                PrivacyGroupTXOptions.builder()
                    .idempotencyKey("idem-1")
                    .gas(HexUint64.of(100_000L))
                    .value(HexUint256.of(7L))
                    .maxPriorityFeePerGas(HexUint256.of(1L))
                    .maxFeePerGas(HexUint256.of(2L))
                    .build())
            .build();

    assertEquals(List.of("me@node1", "you@node2"), input.members());
    assertEquals(Map.of("k", "v", "k2", "v2"), input.properties());
    assertEquals(2, input.configuration().size());
    assertTrue(input.toString().contains("domain=pente"));

    final PrivacyGroupInput parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(input), PrivacyGroupInput.class);
    assertEquals("pente", parsed.domain());
    assertEquals("g1", parsed.name());
    assertEquals(input.members(), parsed.members());
    assertEquals(input.properties(), parsed.properties());

    final PrivacyGroupTXOptions options = parsed.transactionOptions();
    assertEquals("idem-1", options.idempotencyKey());
    assertEquals(HexUint64.of(100_000L), options.gas());
    assertEquals(HexUint256.of(7L), options.value());
    assertEquals(HexUint256.of(1L), options.maxPriorityFeePerGas());
    assertEquals(HexUint256.of(2L), options.maxFeePerGas());
    assertTrue(options.toString().contains("idem-1"));
  }

  @Test
  void privacyGroupInputOmitsEmptyOptionalFields() throws Exception {
    final PrivacyGroupInput input = PrivacyGroupInput.builder("pente").member("me@node1").build();

    final String json = MAPPER.writeValueAsString(input);
    assertEquals("{\"domain\":\"pente\",\"members\":[\"me@node1\"]}", json);
  }

  @Test
  void privacyGroupTxOptionsOmitsEverythingWhenEmpty() throws Exception {
    assertEquals("{}", MAPPER.writeValueAsString(PrivacyGroupTXOptions.builder().build()));
  }

  // ---- PrivacyGroupEVMTXInput ---------------------------------------------

  @Test
  void evmTxInputFlattensTxFieldsAndNestsPublicTxOptions() throws Exception {
    final PrivacyGroupEVMTXInput tx =
        PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .idempotencyKey("idem-2")
            .from("me@node1")
            .to(EthAddress.fromString(ADDRESS))
            .gas(HexUint64.of(500_000L))
            .value(HexUint256.of(3L))
            .input(MAPPER.readTree("{\"amount\":\"10\"}"))
            .function(AbiEntry.function("transfer").build())
            .bytecode(HexBytes.fromString("0xcafe"))
            .publicTxOptions(PublicTxOptions.builder().gas(HexUint64.of(21_000L)).build())
            .build();

    final var node = MAPPER.valueToTree(tx);
    // The EVM transaction fields sit at the top level...
    assertEquals("me@node1", node.get("from").asText());
    assertEquals("0x7a120", node.get("gas").asText());
    assertEquals("10", node.get("input").get("amount").asText());
    assertEquals("transfer", node.get("function").get("name").asText());
    // ...while the base-ledger options stay nested under their own key.
    assertEquals("0x5208", node.get("publicTxOptions").get("gas").asText());

    final PrivacyGroupEVMTXInput parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(tx), PrivacyGroupEVMTXInput.class);
    assertEquals("idem-2", parsed.idempotencyKey());
    assertEquals("pente", parsed.domain());
    assertEquals(HexBytes.fromString(GROUP_ID), parsed.group());
    assertEquals(EthAddress.fromString(ADDRESS), parsed.to());
    assertEquals(HexUint256.of(3L), parsed.value());
    assertEquals(HexBytes.fromString("0xcafe"), parsed.bytecode());
    assertEquals(HexUint64.of(21_000L), parsed.publicTxOptions().gas());
    assertTrue(parsed.toString().contains("domain=pente"));
    assertTrue(parsed.publicTxOptions().toString().contains("gas="));
  }

  @Test
  void evmTxInputOmitsUnsetFields() throws Exception {
    final PrivacyGroupEVMTXInput tx =
        PrivacyGroupEVMTXInput.builder("pente", HexBytes.fromString(GROUP_ID)).build();

    assertEquals(
        "{\"domain\":\"pente\",\"group\":\"" + GROUP_ID + "\"}", MAPPER.writeValueAsString(tx));
  }

  // ---- PrivacyGroupEVMCall ------------------------------------------------

  @Test
  void evmCallFlattensCallOptions() throws Exception {
    final PrivacyGroupEVMCall call =
        PrivacyGroupEVMCall.builder("pente", HexBytes.fromString(GROUP_ID))
            .from("me@node1")
            .to(EthAddress.fromString(ADDRESS))
            .gas(HexUint64.of(50_000L))
            .value(HexUint256.of(0L))
            .input(MAPPER.readTree("[]"))
            .function(AbiEntry.function("balanceOf").build())
            .bytecode(HexBytes.fromString("0xbeef"))
            .block("latest")
            .dataFormat("mode=object,number=string")
            .build();

    final PrivacyGroupEVMCall parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(call), PrivacyGroupEVMCall.class);

    assertEquals("pente", parsed.domain());
    assertEquals(HexBytes.fromString(GROUP_ID), parsed.group());
    assertEquals("me@node1", parsed.from());
    assertEquals(EthAddress.fromString(ADDRESS), parsed.to());
    assertEquals(HexUint64.of(50_000L), parsed.gas());
    assertEquals(HexUint256.of(0L), parsed.value());
    assertTrue(parsed.input().isArray());
    assertEquals("balanceOf", parsed.function().name());
    assertEquals(HexBytes.fromString("0xbeef"), parsed.bytecode());
    assertEquals("latest", parsed.block());
    assertEquals("mode=object,number=string", parsed.dataFormat());
    assertTrue(parsed.toString().contains("domain=pente"));
  }

  @Test
  void evmCallOmitsUnsetFields() throws Exception {
    final PrivacyGroupEVMCall call =
        PrivacyGroupEVMCall.builder("pente", HexBytes.fromString(GROUP_ID)).build();

    assertEquals(
        "{\"domain\":\"pente\",\"group\":\"" + GROUP_ID + "\"}", MAPPER.writeValueAsString(call));
  }

  // ---- messages -----------------------------------------------------------

  @Test
  void messageInputRoundTrips() throws Exception {
    final UUID correlationId = UUID.randomUUID();
    final PrivacyGroupMessageInput message =
        PrivacyGroupMessageInput.builder("pente", HexBytes.fromString(GROUP_ID))
            .correlationId(correlationId)
            .topic("orders")
            .data(MAPPER.readTree("{\"hello\":\"world\"}"))
            .build();

    final PrivacyGroupMessageInput parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(message), PrivacyGroupMessageInput.class);

    assertEquals(correlationId, parsed.correlationId());
    assertEquals("pente", parsed.domain());
    assertEquals(HexBytes.fromString(GROUP_ID), parsed.group());
    assertEquals("orders", parsed.topic());
    assertEquals("world", parsed.data().get("hello").asText());
    assertTrue(parsed.toString().contains("topic=orders"));
  }

  @Test
  void messageInputOmitsUnsetFields() throws Exception {
    final PrivacyGroupMessageInput message =
        PrivacyGroupMessageInput.builder("pente", HexBytes.fromString(GROUP_ID)).build();

    assertEquals(
        "{\"domain\":\"pente\",\"group\":\"" + GROUP_ID + "\"}",
        MAPPER.writeValueAsString(message));
  }

  @Test
  void messageFlattensInputFieldsAlongsideDeliveryMetadata() throws Exception {
    final UUID id = UUID.randomUUID();
    final String json =
        "{\"id\":\""
            + id
            + "\",\"localSequence\":42,\"sent\":\"1700000000000000000\","
            + "\"received\":\"1700000000000000001\",\"node\":\"node2\","
            + "\"domain\":\"pente\",\"group\":\""
            + GROUP_ID
            + "\",\"topic\":\"orders\",\"data\":{\"hello\":\"world\"}}";

    final PrivacyGroupMessage message = MAPPER.readValue(json, PrivacyGroupMessage.class);

    assertEquals(id, message.id());
    assertEquals(42L, message.localSequence());
    assertEquals(Timestamp.ofUnixNano(1700000000000000000L), message.sent());
    assertEquals(Timestamp.ofUnixNano(1700000000000000001L), message.received());
    assertEquals("node2", message.node());
    assertNull(message.correlationId());
    assertEquals("pente", message.domain());
    assertEquals(HexBytes.fromString(GROUP_ID), message.group());
    assertEquals("orders", message.topic());
    assertEquals("world", message.data().get("hello").asText());
    assertTrue(message.toString().contains("localSequence=42"));

    final PrivacyGroupMessage reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(message), PrivacyGroupMessage.class);
    assertEquals(message.id(), reparsed.id());
    assertEquals(message.sent(), reparsed.sent());
  }

  @Test
  void messageNormalizesZeroTimestamps() throws Exception {
    final PrivacyGroupMessage message =
        MAPPER.readValue("{\"sent\":\"0\",\"received\":\"0\"}", PrivacyGroupMessage.class);

    assertNull(message.sent());
    assertNull(message.received());
    assertNull(message.data());
    assertEquals(0L, message.localSequence());
  }

  // ---- listeners ----------------------------------------------------------

  @Test
  void messageListenerRoundTrips() throws Exception {
    final PrivacyGroupMessageListener listener =
        PrivacyGroupMessageListener.builder("orders-listener")
            .started(true)
            .filters(
                PrivacyGroupMessageListenerFilters.builder()
                    .sequenceAbove(10L)
                    .domain("pente")
                    .group(HexBytes.fromString(GROUP_ID))
                    .topic("orders")
                    .build())
            .options(PrivacyGroupMessageListenerOptions.builder().excludeLocal(true).build())
            .build();

    // The builder never sets `created` — that is server-assigned.
    assertNull(listener.created());

    final PrivacyGroupMessageListener parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(listener), PrivacyGroupMessageListener.class);

    assertEquals("orders-listener", parsed.name());
    assertTrue(parsed.started());
    assertEquals(10L, parsed.filters().sequenceAbove());
    assertEquals("pente", parsed.filters().domain());
    assertEquals(HexBytes.fromString(GROUP_ID), parsed.filters().group());
    assertEquals("orders", parsed.filters().topic());
    assertTrue(parsed.options().excludeLocal());
    assertTrue(parsed.toString().contains("orders-listener"));
    assertTrue(parsed.filters().toString().contains("topic=orders"));
    assertTrue(parsed.options().toString().contains("excludeLocal=true"));
  }

  @Test
  void messageListenerNormalizesZeroCreatedAndOmitsUnset() throws Exception {
    final PrivacyGroupMessageListener listener =
        MAPPER.readValue("{\"name\":\"l1\",\"created\":\"0\"}", PrivacyGroupMessageListener.class);

    assertNull(listener.created());
    assertNull(listener.started());
    assertNull(listener.filters());
    assertNull(listener.options());
    assertEquals("{\"name\":\"l1\"}", MAPPER.writeValueAsString(listener));
  }

  @Test
  void listenerFiltersOmitEverythingWhenUnset() throws Exception {
    assertEquals(
        "{}", MAPPER.writeValueAsString(PrivacyGroupMessageListenerFilters.builder().build()));
  }

  // ---- PublicTxOptions ----------------------------------------------------

  @Test
  void publicTxOptionsRoundTripsAndComparesByValue() throws Exception {
    final PublicTxOptions options =
        PublicTxOptions.builder()
            .gas(HexUint64.of(21_000L))
            .value(HexUint256.of(5L))
            .maxPriorityFeePerGas(HexUint256.of(1L))
            .maxFeePerGas(HexUint256.of(2L))
            .build();

    final PublicTxOptions parsed =
        MAPPER.readValue(MAPPER.writeValueAsString(options), PublicTxOptions.class);

    assertEquals(options, parsed);
    assertEquals(options.hashCode(), parsed.hashCode());
    assertEquals(options, options);
    assertFalse(options.equals(PublicTxOptions.builder().build()));
    assertFalse(options.equals("not-options"));
    assertEquals(HexUint64.of(21_000L), parsed.gas());
    assertEquals(HexUint256.of(5L), parsed.value());
    assertEquals(HexUint256.of(1L), parsed.maxPriorityFeePerGas());
    assertEquals(HexUint256.of(2L), parsed.maxFeePerGas());
  }

  @Test
  void publicTxOptionsOmitsEverythingWhenEmpty() throws Exception {
    assertEquals("{}", MAPPER.writeValueAsString(PublicTxOptions.builder().build()));
  }
}
