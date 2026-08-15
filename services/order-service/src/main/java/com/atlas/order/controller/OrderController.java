package com.atlas.order.controller;

import com.atlas.order.client.InventoryClient;
import com.atlas.order.client.PaymentClient;
import com.atlas.order.domain.Order;
import com.atlas.order.dto.CreateOrderRequest;
import com.atlas.order.exception.OrderNotFoundException;
import com.atlas.order.repository.OrderRepository;
import jakarta.validation.Valid;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.UUID;

@RestController
@RequestMapping("/api/orders")
public class OrderController {

    private static final Logger logger = LoggerFactory.getLogger(OrderController.class);
    private final OrderRepository orderRepository;
    private final InventoryClient inventoryClient;
    private final PaymentClient paymentClient;

    public OrderController(OrderRepository orderRepository, InventoryClient inventoryClient, PaymentClient paymentClient) {
        this.orderRepository = orderRepository;
        this.inventoryClient = inventoryClient;
        this.paymentClient = paymentClient;
    }

    @PostMapping
    public ResponseEntity<String> createOrder(@Valid @RequestBody CreateOrderRequest request) {
        logger.info("Creating order for product: {}, quantity: {}", request.getProductId(), request.getQuantity());

        String orderId = "ORD-" + UUID.randomUUID().toString();
        String correlationId = MDC.get("correlationId");
        if (correlationId == null) correlationId = UUID.randomUUID().toString();
        
        String idempotencyKey = "PAY-REQ-" + orderId;

        // Step 1: Reserve Inventory
        inventoryClient.reserveInventory(request.getProductId(), request.getQuantity());

        // Step 2: Authorize Payment
        double amount;
        if (request.getQuantity() == 2) {
            amount = 7777.0; // DECLINE
        } else if (request.getQuantity() == 3) {
            amount = 9999.0; // TIMEOUT
        } else if (request.getQuantity() == 4) {
            amount = 8888.0; // SERVER ERROR
        } else {
            amount = 1000.0 * request.getQuantity();
        }

        try {
            paymentClient.authorizePayment(orderId, amount, idempotencyKey, correlationId);
        } catch (Exception e) {
            logger.error("Payment authorization failed. Attempting compensation.", e);
            try {
                inventoryClient.releaseInventory(request.getProductId(), request.getQuantity());
            } catch (Exception ex) {
                logger.error("Compensation failed to execute cleanly after payment failure", ex);
            }
            if (e instanceof org.springframework.web.server.ResponseStatusException) {
                throw e;
            }
            throw new org.springframework.web.server.ResponseStatusException(HttpStatus.INTERNAL_SERVER_ERROR, "Payment failure", e);
        }

        // Step 3: Create Order
        Order order = new Order();
        order.setOrderId(orderId);
        order.setProductId(request.getProductId());
        order.setQuantity(request.getQuantity());
        order.setStatus("CREATED");

        try {
            orderRepository.save(order);
        } catch (Exception e) {
            logger.error("Order creation failed after successful inventory reservation and payment. Attempting inventory compensation.", e);
            try {
                inventoryClient.releaseInventory(request.getProductId(), request.getQuantity());
            } catch (Exception ex) {
                logger.error("Compensation failed to execute cleanly after order save failure", ex);
            }
            // Payment remains authorized. This is a known limitation documented in M1.4.
            logger.warn("Known limitation: Payment {} remains AUTHORIZED despite order failure.", orderId);
            
            throw new org.springframework.web.server.ResponseStatusException(HttpStatus.INTERNAL_SERVER_ERROR, "Order creation failed", e);
        }

        logger.info("Created order successfully: {}", orderId);
        return ResponseEntity.status(HttpStatus.CREATED).body(order.getOrderId());
    }

    @GetMapping("/{id}")
    public ResponseEntity<Order> getOrder(@PathVariable String id) {
        logger.info("Retrieving order: {}", id);
        
        Order order = orderRepository.findById(id)
                .orElseThrow(() -> new OrderNotFoundException("Order not found: " + id));
                
        return ResponseEntity.ok(order);
    }
}
