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
package io.kaleido.paladin.loader;

import io.kaleido.paladin.logging.PaladinLogging;
import org.apache.logging.log4j.LogManager;
import org.junit.jupiter.api.Test;

import java.io.InputStream;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;

public class PluginJARTest {

    private static final String PLUGIN_JAR = System.getProperty("paladin.testPluginJar");

    private static final String FIXTURE_PACKAGE = "io.kaleido.paladin.loader.fixture.";

    @Test
    void pluginClassesComeFromThePluginJar() throws Exception {
        Class<?> conflict = PluginJAR.classLoaderFor(PLUGIN_JAR).loadClass(FIXTURE_PACKAGE + "Conflict");
        assertEquals("plugin", conflict.getMethod("source").invoke(null));
    }

    @Test
    void coreClassesAreNotVisibleToPlugins() throws Exception {
        ClassLoader pluginLoader = PluginJAR.classLoaderFor(PLUGIN_JAR);
        assertThrows(ClassNotFoundException.class, () -> pluginLoader.loadClass(PluginJARTest.class.getName()));
    }

    @Test
    void loggingIsSharedWithCore() throws Exception {
        ClassLoader pluginLoader = PluginJAR.classLoaderFor(PLUGIN_JAR);
        assertSame(LogManager.class, pluginLoader.loadClass(LogManager.class.getName()));
        assertSame(PaladinLogging.class, pluginLoader.loadClass(PaladinLogging.class.getName()));
    }

    @Test
    void coreServiceFilesAreNotVisibleToPlugins() throws Exception {
        ClassLoader pluginLoader = PluginJAR.classLoaderFor(PLUGIN_JAR);
        List<String> providers = new ArrayList<>();
        for (URL url : Collections.list(pluginLoader.getResources("META-INF/services/" + FIXTURE_PACKAGE + "FixtureService"))) {
            try (InputStream is = url.openStream()) {
                providers.add(new String(is.readAllBytes(), StandardCharsets.UTF_8).trim());
            }
        }
        assertEquals(List.of(FIXTURE_PACKAGE + "PluginProvider"), providers);
    }

    @Test
    void pluginInstancesFromOneJarShareAClassLoader() throws Exception {
        assertSame(PluginJAR.classLoaderFor(PLUGIN_JAR), PluginJAR.classLoaderFor(PLUGIN_JAR));
    }

    @Test
    void pluginStartsAndStopsWithItsOwnContextClassLoader() throws Exception {
        String instanceId = UUID.randomUUID().toString();
        PluginJAR plugin = new PluginJAR("unused", new PluginInfo("unused", "DOMAIN", "fixture", instanceId),
                (id, p, t) -> {}, PLUGIN_JAR, FIXTURE_PACKAGE + "FixturePlugin");
        plugin.loadAndStart();
        plugin.stop();

        ClassLoader pluginLoader = PluginJAR.classLoaderFor(PLUGIN_JAR);
        Class<?> fixturePlugin = pluginLoader.loadClass(FIXTURE_PACKAGE + "FixturePlugin");
        assertSame(pluginLoader, fixturePlugin.getField("startContextClassLoader").get(null));
        assertSame(pluginLoader, fixturePlugin.getField("stopContextClassLoader").get(null));
    }
}
