-- needs to be sqlite compatible

-- wait for up to 5 seconds before failing concurrent access
PRAGMA busy_timeout = 5000;

-- managed by inference-server-manager
CREATE TABLE servers (
	server_id INTEGER PRIMARY KEY,
	pid INTEGER NOT NULL,
	port INTEGER NOT NULL,
	model TEXT NOT NULL
);

-- managed by tracker
CREATE TABLE nodes (
	ip TEXT NOT NULL PRIMARY KEY,
	port INTEGER NOT NULL,
	last_seen DATETIME NOT NULL,
	hardware_model TEXT,
	max_size INTEGER,
	battery REAL,
	temperature REAL
);

CREATE TABLE server_nodes (
    server_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    FOREIGN KEY (server_id) REFERENCES servers(server_id) ON DELETE CASCADE,
    FOREIGN KEY (node_id) REFERENCES nodes(node_id) ON DELETE CASCADE,
    PRIMARY KEY (server_id, node_id)
);