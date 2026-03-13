CREATE TABLE servers (
	server_id INTEGER PRIMARY KEY,
	pid INTEGER NOT NULL,
	port INTEGER NOT NULL,
	model TEXT NOT NULL,	
);

CREATE TABLE server_nodes (
    server_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    FOREIGN KEY (server_id) REFERENCES servers(server_id),
    FOREIGN KEY (node_id) REFERENCES nodes(node_id),
    PRIMARY KEY (server_id, node_id),
    ON DELETE CASCADE
);