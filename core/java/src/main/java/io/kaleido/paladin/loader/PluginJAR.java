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

import io.kaleido.paladin.logging.PaladinLogging;
import org.apache.logging.log4j.Logger;

import java.io.File;
import java.io.UncheckedIOException;
import java.lang.reflect.Method;
import java.net.MalformedURLException;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

public class PluginJAR extends Plugin {

    private static final Logger LOGGER = PaladinLogging.getLogger(PluginJAR.class);

    // Every plugin instance loaded from a jar shares one class loader, so the jar's classes are loaded once.
    // A plugin's classes stay in use by the threads it started, so each class loader is kept for the life
    // of the process.
    private static final Map<String, ClassLoader> jarClassLoaders = new ConcurrentHashMap<>();

    private final String libName;
    private final String className;
    private ClassLoader classLoader;
    private Object pluginImpl;
    private Method stopInstanceMethod;

    PluginJAR(String grpcTarget, PluginInfo info, PluginStopped onStop, String libName, String className) {
        super(grpcTarget, info, onStop);
        this.libName = libName;
        this.className = className;
    }

    @Override
    public synchronized void loadAndStart() throws Exception {
        LOGGER.info("loading JAR plugin {} libName={}", className, libName);
        classLoader = this.getClass().getClassLoader();
        if (libName != null && !libName.isBlank()) {
            classLoader = classLoaderFor(libName);
        }
        // Libraries look classes up through the thread's context class loader, and threads inherit it from
        // the thread that creates them, so the plugin starts and stops with its own class loader in place.
        Thread thread = Thread.currentThread();
        ClassLoader previous = thread.getContextClassLoader();
        thread.setContextClassLoader(classLoader);
        try {
            Class<?> clazz = classLoader.loadClass(className);
            pluginImpl = clazz.getDeclaredConstructor().newInstance();
            Method startInstanceMethod = clazz.getMethod("startInstance", String.class, String.class);
            stopInstanceMethod = clazz.getMethod("stopInstance", String.class);
            LOGGER.info("loaded JAR plugin {} pluginId={}", pluginImpl.getClass().getName(), info.instanceId());
            startInstanceMethod.invoke(pluginImpl, grpcTarget, info.instanceId());
        } finally {
            thread.setContextClassLoader(previous);
        }
        LOGGER.info("started JAR plugin {} pluginId={}", pluginImpl.getClass().getName(), info.instanceId());
    }

    static ClassLoader classLoaderFor(String libName) {
        return jarClassLoaders.computeIfAbsent(libName, n -> {
            try {
                return new PluginClassLoader(new File(n).toURI().toURL(), PluginJAR.class.getClassLoader());
            } catch (MalformedURLException e) {
                throw new UncheckedIOException(e);
            }
        });
    }

    @Override
    public synchronized void stop() throws Exception {
        if (stopInstanceMethod != null) {
            Thread thread = Thread.currentThread();
            ClassLoader previous = thread.getContextClassLoader();
            thread.setContextClassLoader(classLoader);
            try {
                stopInstanceMethod.invoke(pluginImpl, info.instanceId());
            } finally {
                thread.setContextClassLoader(previous);
            }
        }
        onStop.pluginStopped(info.instanceId(), this, null);
    }

}
