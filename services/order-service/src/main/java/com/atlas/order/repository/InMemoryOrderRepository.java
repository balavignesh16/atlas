package com.atlas.order.repository;

import com.atlas.order.domain.Order;
import org.springframework.stereotype.Repository;

import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;

@Repository
public class InMemoryOrderRepository implements OrderRepository {

    private final ConcurrentMap<String, Order> store = new ConcurrentHashMap<>();

    @Override
    public Order save(Order order) {
        if (order.getOrderId() == null) {
            order.setOrderId("ORD-" + UUID.randomUUID().toString());
        }
        store.put(order.getOrderId(), order);
        return order;
    }

    @Override
    public Optional<Order> findById(String orderId) {
        return Optional.ofNullable(store.get(orderId));
    }
}
