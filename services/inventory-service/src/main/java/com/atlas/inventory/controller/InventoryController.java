package com.atlas.inventory.controller;

import com.atlas.inventory.domain.InventoryItem;
import com.atlas.inventory.dto.ReleaseRequest;
import com.atlas.inventory.dto.ReserveRequest;
import com.atlas.inventory.dto.ReserveResponse;
import com.atlas.inventory.exception.ProductNotFoundException;
import com.atlas.inventory.repository.InventoryRepository;
import jakarta.validation.Valid;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/inventory")
public class InventoryController {

    private final InventoryRepository inventoryRepository;

    public InventoryController(InventoryRepository inventoryRepository) {
        this.inventoryRepository = inventoryRepository;
    }

    @GetMapping("/{productId}")
    public InventoryItem getInventory(@PathVariable String productId) {
        return inventoryRepository.findByProductId(productId)
                .orElseThrow(() -> new ProductNotFoundException("Product not found"));
    }

    @PostMapping("/{productId}/reserve")
    public ReserveResponse reserveInventory(@PathVariable String productId, @Valid @RequestBody ReserveRequest request) {
        InventoryItem updatedItem = inventoryRepository.reserve(productId, request.getQuantity());
        return new ReserveResponse(updatedItem.getProductId(), request.getQuantity(), updatedItem.getAvailableQuantity());
    }

    @PostMapping("/{productId}/release")
    public void releaseInventory(@PathVariable String productId, @Valid @RequestBody ReleaseRequest request) {
        inventoryRepository.release(productId, request.getQuantity());
    }
}
