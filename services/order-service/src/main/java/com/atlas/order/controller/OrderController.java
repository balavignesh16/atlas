package com.atlas.order.controller;

import com.atlas.order.domain.Order;
import com.atlas.order.dto.CreateOrderRequest;
import com.atlas.order.exception.OrderNotFoundException;
import com.atlas.order.repository.OrderRepository;
import jakarta.validation.Valid;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.net.URI;
import java.time.Instant;

@RestController
@RequestMapping("/api/orders")
public class OrderController {

    private static final Logger logger = LoggerFactory.getLogger(OrderController.class);
    private final OrderRepository orderRepository;

    public OrderController(OrderRepository orderRepository) {
        this.orderRepository = orderRepository;
    }

    @PostMapping
    public ResponseEntity<Order> createOrder(@Valid @RequestBody CreateOrderRequest request) {
        logger.info("Creating order for product: {}, quantity: {}", request.getProductId(), request.getQuantity());

        Order order = new Order();
        order.setProductId(request.getProductId());
        order.setQuantity(request.getQuantity());
        order.setStatus("CREATED");
        order.setCreatedAt(Instant.now());

        Order savedOrder = orderRepository.save(order);
        
        logger.info("Created order successfully: {}", savedOrder.getOrderId());
        return ResponseEntity
                .created(URI.create("/api/orders/" + savedOrder.getOrderId()))
                .body(savedOrder);
    }

    @GetMapping("/{id}")
    public ResponseEntity<Order> getOrder(@PathVariable String id) {
        logger.info("Retrieving order: {}", id);
        
        Order order = orderRepository.findById(id)
                .orElseThrow(() -> new OrderNotFoundException("Order not found: " + id));
                
        return ResponseEntity.ok(order);
    }
}
