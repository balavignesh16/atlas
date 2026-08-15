package com.atlas.order.client;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatusCode;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientResponseException;
import org.springframework.web.client.ResourceAccessException;
import org.springframework.web.server.ResponseStatusException;
import org.springframework.http.HttpStatus;

import java.util.Map;

@Component
public class InventoryClient {
    private static final Logger logger = LoggerFactory.getLogger(InventoryClient.class);
    private final RestClient restClient;

    public InventoryClient(RestClient.Builder builder, @Value("${inventory.service.url:http://localhost:8085}") String inventoryUrl) {
        this.restClient = builder.baseUrl(inventoryUrl).build();
    }

    public void reserveInventory(String productId, int quantity) {
        String correlationId = MDC.get("correlationId");
        try {
            restClient.post()
                    .uri("/api/inventory/{productId}/reserve", productId)
                    .header("X-Correlation-ID", correlationId)
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(Map.of("quantity", quantity))
                    .retrieve()
                    .onStatus(HttpStatusCode::is4xxClientError, (request, response) -> {
                        if (response.getStatusCode().value() == 409) {
                            throw new ResponseStatusException(HttpStatus.CONFLICT, "Insufficient inventory");
                        } else if (response.getStatusCode().value() == 404) {
                            throw new ResponseStatusException(HttpStatus.NOT_FOUND, "Product not found");
                        }
                        throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "Invalid inventory request");
                    })
                    .onStatus(HttpStatusCode::is5xxServerError, (request, response) -> {
                        throw new ResponseStatusException(HttpStatus.BAD_GATEWAY, "Inventory service error");
                    })
                    .toBodilessEntity();
        } catch (ResourceAccessException e) {
            // Connection refused -> 503, Timeout -> 504
            if (e.getMessage() != null && e.getMessage().toLowerCase().contains("timeout")) {
                throw new ResponseStatusException(HttpStatus.GATEWAY_TIMEOUT, "Inventory service timeout", e);
            }
            throw new ResponseStatusException(HttpStatus.SERVICE_UNAVAILABLE, "Inventory service unavailable", e);
        }
    }

    public void releaseInventory(String productId, int quantity) {
        String correlationId = MDC.get("correlationId");
        try {
            restClient.post()
                    .uri("/api/inventory/{productId}/release", productId)
                    .header("X-Correlation-ID", correlationId)
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(Map.of("quantity", quantity))
                    .retrieve()
                    .toBodilessEntity();
            logger.info("Successfully released inventory for product: {}, quantity: {}", productId, quantity);
        } catch (Exception e) {
            logger.error("Failed to release inventory during compensation for product: {}, quantity: {}", productId, quantity, e);
            // We don't throw here for compensation, just log.
        }
    }
}
