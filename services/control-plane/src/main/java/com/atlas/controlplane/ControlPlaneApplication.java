package com.atlas.controlplane;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import jakarta.annotation.PreDestroy;

@SpringBootApplication
public class ControlPlaneApplication {
    private static final Logger logger = LoggerFactory.getLogger(ControlPlaneApplication.class);

    public static void main(String[] args) {
        SpringApplication.run(ControlPlaneApplication.class, args);
        logger.info("ATLAS Control Plane started successfully");
    }

    @PreDestroy
    public void onShutdown() {
        logger.info("ATLAS Control Plane is shutting down gracefully");
    }
}
