CREATE DATABASE blocks;

\c blocks

CREATE TABLE blocks (
    network VARCHAR,
    block_number INTEGER,
    block_timestamp BIGINT,
    collator_address CHAR(47) NOT NULL,
    extrinsics_count INTEGER NOT NULL,
    weight BIGINT,
    weight_ratio FLOAT NOT NULL,
    PRIMARY KEY ( network, block_number )
);
