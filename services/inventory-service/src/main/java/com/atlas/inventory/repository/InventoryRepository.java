package com.atlas.inventory.repository;

import com.atlas.inventory.domain.InventoryItem;
import com.atlas.inventory.exception.InsufficientInventoryException;
import com.atlas.inventory.exception.ProductNotFoundException;
import org.springframework.stereotype.Repository;

import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

@Repository
public class InventoryRepository {
    private final ConcurrentHashMap<String, InventoryItem> inventory = new ConcurrentHashMap<>();

    public InventoryRepository() {
        // Initial seed data as per M1.3 requirements
        inventory.put("P100", new InventoryItem("P100", 10));
        inventory.put("P200", new InventoryItem("P200", 5));
        inventory.put("P300", new InventoryItem("P300", 0));
    }

    public Optional<InventoryItem> findByProductId(String productId) {
        InventoryItem item = inventory.get(productId);
        if (item == null) {
            return Optional.empty();
        }
        return Optional.of(new InventoryItem(item.getProductId(), item.getAvailableQuantity())); // return a copy to prevent unsynchronized modification
    }

    public InventoryItem reserve(String productId, int quantity) {
        InventoryItem[] result = new InventoryItem[1];
        
        inventory.compute(productId, (key, existingItem) -> {
            if (existingItem == null) {
                throw new ProductNotFoundException("Product not found: " + productId);
            }
            if (existingItem.getAvailableQuantity() < quantity) {
                throw new InsufficientInventoryException("Insufficient inventory for product: " + productId);
            }
            
            int newQuantity = existingItem.getAvailableQuantity() - quantity;
            existingItem.setAvailableQuantity(newQuantity);
            result[0] = new InventoryItem(existingItem.getProductId(), existingItem.getAvailableQuantity());
            return existingItem;
        });
        
        return result[0];
    }

    public InventoryItem release(String productId, int quantity) {
        InventoryItem[] result = new InventoryItem[1];
        
        inventory.compute(productId, (key, existingItem) -> {
            if (existingItem == null) {
                throw new ProductNotFoundException("Product not found: " + productId);
            }
            
            int newQuantity = existingItem.getAvailableQuantity() + quantity;
            existingItem.setAvailableQuantity(newQuantity);
            result[0] = new InventoryItem(existingItem.getProductId(), existingItem.getAvailableQuantity());
            return existingItem;
        });
        
        return result[0];
    }
    
    // For testing
    public void reset() {
        inventory.clear();
        inventory.put("P100", new InventoryItem("P100", 10));
        inventory.put("P200", new InventoryItem("P200", 5));
        inventory.put("P300", new InventoryItem("P300", 0));
    }
}
