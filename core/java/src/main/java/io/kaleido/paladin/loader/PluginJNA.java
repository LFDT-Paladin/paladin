/*
 * Copyright © 2024 Kaleido, Inc.
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

import com.sun.jna.Library;
import com.sun.jna.Native;
import io.kaleido.paladin.logging.PaladinLogging;
import org.apache.logging.log4j.Logger;

import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class PluginJNA extends Plugin {

    private static final Logger LOGGER = PaladinLogging.getLogger(PluginJNA.class);

    // A loaded library keeps its own threads and signal handlers active in this process even
    // when no plugin instance is running in it, so unloading it is unsafe. JNA unloads a library
    // once its proxy is unreachable; this map keeps each one loaded for the life of the process.
    private static final Map<String, PluginCShared> loadedLibs = new ConcurrentHashMap<>();

    private final String libName;

    private PluginCShared lib;

    interface PluginCShared extends Library {
        int Run(String grpcTarget, String pluginId);
        void Stop(String pluginId);
    }

    PluginJNA(String grpcTarget, PluginInfo info, PluginStopped onStop, String libName) throws UnsatisfiedLinkError {
        super(grpcTarget, info, onStop);
        this.libName = libName;
    }


    @Override
    public synchronized void stop() throws Exception {
        lib.Stop(info.instanceId());
    }

    @Override
    public synchronized void loadAndStart() throws Exception {
        LOGGER.info("Loading plugin via JNA: {}", libName);
        lib = loadedLibs.computeIfAbsent(libName, n -> Native.load(n, PluginCShared.class));
        CompletableFuture.runAsync(() -> {
            LOGGER.info("starting {} {} [{}]", info.pluginType(), info.name(), info.instanceId());
            int rc = lib.Run(this.grpcTarget, info.instanceId());
            if (rc != 0) {
                throw new RuntimeException("Plugin returned RC=%d".formatted(rc));
            }
        }, Executors.newSingleThreadExecutor())
                .whenComplete((voidResult, t) -> onStop.pluginStopped(info.instanceId(), this, t));
    }
}
