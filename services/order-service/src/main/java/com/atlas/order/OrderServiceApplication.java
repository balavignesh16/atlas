package com.atlas.order;

import jakarta.annotation.PreDestroy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class OrderServiceApplication {

    private static final Logger logger = LoggerFactory.getLogger(OrderServiceApplication.class);

    public static void main(String[] args) {
        SpringApplication.run(OrderServiceApplication.class, args);
        logger.info("ATLAS Order Service started successfully");
    }

    @PreDestroy
    public void onExit() {
        logger.info("ATLAS Order Service is shutting down gracefully");
    }
}
