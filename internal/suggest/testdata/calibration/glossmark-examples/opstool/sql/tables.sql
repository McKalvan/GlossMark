-- run once on db01. ALTERs are at the bottom, run them in order.

-- @gloss represents wh:Item conf=0.41
CREATE TABLE tbl_master (
  item        VARCHAR(14) PRIMARY KEY,
  descr       VARCHAR(80) NOT NULL,
  uom         CHAR(2)     NOT NULL DEFAULT 'EA',
  cls         VARCHAR(4)  NULL
);

-- @gloss represents wh:Bin conf=0.41
CREATE TABLE tbl_loc (
  loc         VARCHAR(10) PRIMARY KEY,
  zone        CHAR(1)     NOT NULL,
  min_qty     INT         NOT NULL DEFAULT 0,
  max_qty     INT         NOT NULL DEFAULT 0
);

-- @gloss represents wh:StockOnHand conf=0.9
CREATE TABLE tbl_data (
  loc         VARCHAR(10) NOT NULL REFERENCES tbl_loc(loc),
  item        VARCHAR(14) NOT NULL REFERENCES tbl_master(item),
  qty         INT         NOT NULL DEFAULT 0,
  PRIMARY KEY (loc, item)
);

-- @gloss represents wh:CustomerOrder conf=0.45
CREATE TABLE tbl_ord (
  ord_no      VARCHAR(12) NOT NULL,
  line_no     INT         NOT NULL,
  item        VARCHAR(14) NOT NULL,
  qty         INT         NOT NULL,
  cutoff      CHAR(5)     NOT NULL,
  status      CHAR(1)     NOT NULL DEFAULT 'N',
  PRIMARY KEY (ord_no, line_no)
);

CREATE TABLE tbl_queue (
  id          INT AUTO_INCREMENT PRIMARY KEY,
  batch       VARCHAR(20) NULL,
  ord_no      VARCHAR(12) NULL,
  line_no     INT         NULL,
  loc_from    VARCHAR(10) NULL,
  loc         VARCHAR(10) NOT NULL,
  item        VARCHAR(14) NOT NULL,
  qty         INT         NOT NULL,
  qty_done    INT         NULL,
  kind        CHAR(1)     NOT NULL,
  done        TINYINT     NOT NULL DEFAULT 0,
  ts          DATETIME    NOT NULL
);

-- @gloss represents wh:CountVariance conf=0.8
CREATE TABLE tbl_cnt (
  id          INT AUTO_INCREMENT PRIMARY KEY,
  loc         VARCHAR(10) NOT NULL,
  item        VARCHAR(14) NOT NULL,
  found       INT         NOT NULL,
  sys_qty     INT         NOT NULL,
  diff        DECIMAL(8,4) NOT NULL,
  ts          DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- @gloss represents wh:ReturnAuthorization conf=0.41
CREATE TABLE tbl_rma (
  rma_no      VARCHAR(12) PRIMARY KEY,
  cust        VARCHAR(12) NOT NULL,
  status      VARCHAR(6)  NOT NULL DEFAULT 'OPEN'
);

CREATE TABLE tbl_rma_vendor (
  rma_no      VARCHAR(12) NOT NULL,
  item        VARCHAR(14) NOT NULL,
  qty         INT         NOT NULL
);

CREATE TABLE tbl_log (
  id          INT AUTO_INCREMENT PRIMARY KEY,
  kind        CHAR(3)     NOT NULL,
  loc         VARCHAR(10) NULL,
  item        VARCHAR(14) NULL,
  qty         INT         NULL,
  who         VARCHAR(20) NULL,
  ts          DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE tbl_master ADD COLUMN weight_g INT NULL;
ALTER TABLE tbl_queue ADD INDEX ix_batch (batch, done);
