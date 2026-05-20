package io.konveyor.demo.ordermanagement.repository;

import io.konveyor.demo.ordermanagement.model.Customer;
import org.springframework.data.repository.PagingAndSortingRepository;

public interface CustomerRepository extends PagingAndSortingRepository<Customer, Long> {

}
