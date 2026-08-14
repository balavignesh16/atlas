package com.atlas.order.repository;

import com.atlas.order.domain.Order;
import java.util.Optional;

public interface OrderRepository {
    Order save(Order order);
    Optional<Order> findById(String orderId);
}
