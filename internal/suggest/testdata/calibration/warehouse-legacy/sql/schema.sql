-- @gloss represents wh:Location
CREATE TABLE tbl_loc (id INT PRIMARY KEY, zone TEXT, qty INT);

-- @gloss represents wh:Order
CREATE TABLE tbl_ord (id INT PRIMARY KEY, status TEXT);

-- @gloss represents wh:ReturnAuthorization
CREATE TABLE tbl_rma (id INT PRIMARY KEY, customer TEXT);

CREATE TABLE tbl_rma_vendor (rma INT, vendor TEXT);

CREATE TABLE tbl_data (k TEXT, v TEXT);

CREATE TABLE tbl_log (sku TEXT, qty INT);

CREATE INDEX tbl_rma_customer_idx ON tbl_rma (customer);
