package com.atlas.gateway.controller;

import com.atlas.gateway.filter.CorrelationIdFilter;
import org.springframework.http.HttpHeaders;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.client.RestClient;

@RestController
@RequestMapping("/api/orders")
public class GatewayController {

    private final RestClient restClient;

    public GatewayController(RestClient restClient) {
        this.restClient = restClient;
    }

    @PostMapping
    public ResponseEntity<String> createOrder(
            @RequestBody(required = false) String body,
            @RequestAttribute(CorrelationIdFilter.CORRELATION_ID_KEY) String correlationId) {
        
        RestClient.RequestBodySpec request = restClient.post()
                .uri("/api/orders")
                .header(CorrelationIdFilter.CORRELATION_ID_HEADER, correlationId);
                
        if (body != null) {
            request.header(HttpHeaders.CONTENT_TYPE, "application/json");
            request.body(body);
        }
        
        return request.retrieve().toEntity(String.class);
    }

    @GetMapping("/{id}")
    public ResponseEntity<String> getOrder(
            @PathVariable String id,
            @RequestAttribute(CorrelationIdFilter.CORRELATION_ID_KEY) String correlationId) {
        
        return restClient.get()
                .uri("/api/orders/{id}", id)
                .header(CorrelationIdFilter.CORRELATION_ID_HEADER, correlationId)
                .retrieve()
                .toEntity(String.class);
    }
}
