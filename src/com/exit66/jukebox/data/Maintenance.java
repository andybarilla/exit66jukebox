/*
 * Maintenance.java
 *
 * Created on April 12, 2006, 9:25 AM
 *
 * To change this template, choose Tools | Template Manager
 * and open the template in the editor.
 */

package com.exit66.jukebox.data;

import com.exit66.jukebox.Options;

import java.sql.ResultSet;
import java.sql.SQLException;
/**
 *
 * @author andyb
 */
public class Maintenance {
    
    BVDatabase conn;
    /** Creates a new instance of Maintenance */
    public Maintenance() {
        conn = new BVDatabase();
    }
    
     /**
     *
     * Verifies the existance of the database.	 If it doesn't exist then creates the database.
     * Calls the UpdateDatabase routine.
     *
     * @see	updateDatabase()
     *
     */
    public void verifyDatabase() {
       
        try {
            
            conn.openConnection();
            
            ResultSet result = conn.retrieveData("SELECT * FROM library");
            
            if (result == null) {
            	createDatabase();
            	conn.closeConnection();
            	return;
            }
            try {
            	result.close();
            }
            catch (SQLException e) {
            	System.err.println(e);
            }
            
            if (Options.getDatabaseVersion() == 0) {
                
                Options.setDefaultFile("index.html");
                Options.setDatabaseVersion(5);
                Options.saveOptions();
                
            }
            
            updateDatabase();
            
            conn.executeStatement("UPDATE library SET scan_status = 0");
            
        } finally {
            
            conn.closeConnection();
            
        }
        
    }
    
    /**
     *
     * Creates the tables in the database
     *
     */
    public void createDatabase() {
        
        try {
            
            conn.openConnection();
            
            conn.executeStatement("CREATE TABLE library (library_id INT NOT NULL IDENTITY, library_path VARCHAR_IGNORECASE(255), scan_status INT)");
            
            conn.executeStatement("CREATE TABLE library_failed_files (library_id INT NOT NULL, full_file_name VARCHAR_IGNORECASE(255), file_name VARCHAR_IGNORECASE(255), CONSTRAINT lff_pk PRIMARY KEY (library_id, file_name))");
            
            conn.executeStatement("CREATE TABLE artist (artist_id INT NOT NULL IDENTITY, artist_name VARCHAR_IGNORECASE(255), artist_order_name VARCHAR_IGNORECASE(255), album_count INT NOT NULL, track_count INT NOT NULL)");
            
            conn.executeStatement("CREATE TABLE album (album_id INT NOT NULL IDENTITY, album_name VARCHAR_IGNORECASE(255), multiple_artist_id INT, track_count INT NOT NULL)");
            
            conn.executeStatement("CREATE TABLE artistalbumlink (artist_id INT NOT NULL, album_id INT NOT NULL, CONSTRAINT aalink_pk PRIMARY KEY(artist_id, album_id))");
            
            conn.executeStatement("CREATE TABLE track (track_id INT NOT NULL IDENTITY, library_id INT, file_name VARCHAR_IGNORECASE(255), track_name VARCHAR_IGNORECASE(255), track_number INT, artist_id INT, album_id INT, content_type_id INT, playcount INT NOT NULL, track_time VARCHAR(8))");
            
            conn.executeStatement("CREATE TABLE content_type (content_type_id INT NOT NULL IDENTITY, content_type VARCHAR_IGNORECASE(255))");
            
            conn.executeStatement("CREATE TABLE request_queue (track_id INT NOT NULL, session_id VARCHAR_IGNORECASE(36), play_order INT NOT NULL, CONSTRAINT requestqueue_pk PRIMARY KEY(track_id, session_id))");
            
            conn.executeStatement("CREATE TABLE playlist (playlist_id INT NOT NULL IDENTITY, playlist_name VARCHAR_IGNORECASE(255), activated INT)");
            
            conn.executeStatement("CREATE TABLE playlist_track (playlist_id INT NOT NULL, track_id INT NOT NULL, play_order INT NOT NULL, playcount INT NOT NULL, CONSTRAINT playlist_track_pl PRIMARY KEY(playlist_id, track_id))");
            
            conn.executeStatement("CREATE TABLE playlist_rule (playlist_rule_id INT NOT NULL IDENTITY, playlist_id INT NOT NULL, rule_type VARCHAR_IGNORECASE(255) NOT NULL, rule_value VARCHAR_IGNORECASE(255) NOT NULL)");
            
            conn.executeStatement("CREATE VIEW vw_albumartistcount AS SELECT album_id, COUNT(*) AS artist_count FROM artistalbumlink GROUP BY album_id");
            
            conn.executeStatement("CREATE TABLE history (track_id INT NOT NULL, session_id VARCHAR_IGNORECASE(36), entry DATETIME)");
            
            conn.executeStatement("CREATE INDEX idx_artist_name ON artist (artist_name)");
            
            conn.executeStatement("CREATE INDEX idx_artist_order_name ON artist (artist_order_name)");
            
            conn.executeStatement("CREATE INDEX idx_album_name ON album (album_name)");
            
            conn.executeStatement("CREATE INDEX idx_track_name ON track (track_name)");
            
            conn.executeStatement("ALTER TABLE album ADD artist_count INT");
            
            conn.executeStatement("ALTER TABLE track ADD FOREIGN KEY (artist_id) REFERENCES artist (artist_id)");
            
            conn.executeStatement("ALTER TABLE track ADD FOREIGN KEY (album_id) REFERENCES album (album_id)");
            
            conn.executeStatement("CREATE INDEX idx_request_track ON request_queue (track_id)");
            
            conn.executeStatement("CREATE INDEX idx_request_play_order ON request_queue (play_order)");
            
            conn.executeStatement("CREATE INDEX idx_history_track ON history (track_id)");
            
            conn.executeStatement("CREATE INDEX idx_history_entry ON history (entry)");
            
            conn.executeStatement("CREATE INDEX idx_playlist_rule ON playlist_rule (playlist_id)");
            
        } finally {
            
            conn.closeConnection();
            
        }
        
    }
    
    /**
     *
     * Checks the version of the database and updates it if necessary with table changes
     *
     */
    public void updateDatabase() {
        
        
        if (Options.getDatabaseVersion() < 2) {
            
            try {
                                
                conn.openConnection();
                
                conn.executeStatement("ALTER TABLE library ADD COLUMN scan_status INT");
                
                conn.executeStatement("CREATE TABLE library_failed_files (library_id INT NOT NULL, file_name VARCHAR_IGNORECASE)");
                
                Options.setDatabaseVersion(2);
                Options.saveOptions();
                
            } finally {
            
            conn.closeConnection();
            
            }
            
        }
        
        if (Options.getDatabaseVersion() < 3) {
            try {
                                
                conn.openConnection();
        
                conn.executeStatement("CREATE INDEX idx_artist_name ON artist (artist_name)");
                
                conn.executeStatement("CREATE INDEX idx_album_name ON album (album_name)");
                
                conn.executeStatement("CREATE INDEX idx_track_name ON track (track_name)");
                
                conn.executeStatement("ALTER TABLE album ADD artist_count INT");
                
                conn.executeStatement("ALTER TABLE track ADD FOREIGN KEY (artist_id) REFERENCES artist (artist_id)");
                
                conn.executeStatement("ALTER TABLE track ADD FOREIGN KEY (album_id) REFERENCES album (album_id)");
                
                conn.executeStatement("CREATE INDEX idx_request_track ON request_queue (track_id)");
                
                conn.executeStatement("CREATE INDEX idx_request_play_order ON request_queue (play_order)");
                
                conn.executeStatement("CREATE INDEX idx_history_track ON history (track_id)");
                
                conn.executeStatement("CREATE INDEX idx_history_entry ON history (entry)");
                
                AlbumCollection list = new AlbumCollection();
                list.updateAlbumArtistCount();
                
                Options.setDefaultFile("index.html");
                Options.setDatabaseVersion(3);
                Options.saveOptions();
                
            } finally {
            
            	conn.closeConnection();
            
            }
        }
        
        if (Options.getDatabaseVersion() < 4) {
        	try {
        		conn.openConnection();
        		
        		conn.executeStatement("CREATE TABLE playlist_rule (playlist_rule_id INT NOT NULL IDENTITY PRIMARY KEY, playlist_id INT NOT NULL, rule_type VARCHAR_IGNORECASE NOT NULL, rule_value VARCHAR_IGNORECASE NOT NULL)");
                
                conn.executeStatement("CREATE INDEX idx_playlist_rule ON playlist_rule (playlist_id)");
            
                Options.setDefaultFile("index.html");
                Options.setDatabaseVersion(4);
                Options.saveOptions();
                
            } finally {
            
            	conn.closeConnection();
            
            }
        }
        
        if (Options.getDatabaseVersion() < 5) {
        	try {
        		conn.openConnection();
        		
        		conn.executeStatement("CREATE INDEX idx_artist_order_name ON artist (artist_order_name)");
        		
                Options.setDefaultFile("index.html");
                Options.setDatabaseVersion(5);
                Options.saveOptions();
                
            } finally {
            
            	conn.closeConnection();
            
            }
        }
        
    }
    
    /**
     *
     * Cleans out the database by deleteing extraneous records
     *
     */
    public void cleanDatabase() {
        
        try {
            
            conn.openConnection();
            
            conn.executeStatement("DELETE FROM track WHERE library_id NOT IN (SELECT library_id FROM library)");
            
            conn.executeStatement("DELETE FROM track WHERE artist_id NOT IN (SELECT artist_id FROM artist)");
            
            conn.executeStatement("DELETE FROM track WHERE album_id NOT IN (SELECT album_id FROM album)");
            
            conn.executeStatement("DELETE FROM album WHERE album_id NOT IN (SELECT album_id FROM artistalbumlink)");
            
            conn.executeStatement("DELETE FROM album WHERE album_id NOT IN (SELECT album_id FROM track)");
            
            conn.executeStatement("DELETE FROM artist WHERE artist_id NOT IN (SELECT artist_id FROM artistalbumlink) AND artist_id NOT IN (SELECT multiple_artist_id FROM album)");
            
            conn.executeStatement("DELETE FROM artist WHERE artist_id NOT IN (SELECT artist_id FROM track) AND artist_id NOT IN (SELECT multiple_artist_id FROM album)");
            
            conn.executeStatement("DELETE FROM content_type WHERE content_type_id NOT IN (SELECT content_type_id FROM track)");
            
            conn.executeStatement("DELETE FROM artistalbumlink WHERE artist_id NOT IN (SELECT artist_id FROM artist) AND album_id NOT IN (SELECT album_id FROM album)");
            
            conn.executeStatement("DELETE FROM playlist_track WHERE track_id NOT IN (SELECT track_id FROM track)");
            
            conn.executeStatement("DELETE FROM library_failed_files WHERE library_id NOT IN (SELECT library_id FROM library)");
            
            conn.executeStatement("DELETE FROM library_failed_files WHERE file_name IN (SELECT file_name FROM track)");
            
            conn.executeStatement("UPDATE artist SET track_count = (SELECT COUNT(*) FROM artist AS a, track AS t WHERE (t.artist_id = a.artist_id OR t.album_id IN (SELECT album_id FROM album AS al WHERE al.multiple_artist_id = a.artist_id)) AND a.artist_id = artist.artist_id GROUP BY a.artist_id)");
            
            conn.executeStatement("UPDATE album SET track_count = (SELECT COUNT(*) AS total FROM track WHERE track.album_id = album.album_id GROUP BY album_id)");
            
            conn.executeStatement("UPDATE artist SET album_count = (SELECT COUNT(*) AS total FROM artist AS a, album AS al WHERE (a.artist_id = al.multiple_artist_id OR al.album_id IN (SELECT album_id FROM artistalbumlink AS aa WHERE aa.artist_id = a.artist_id)) AND a.artist_id = artist.artist_id GROUP BY a.artist_id)");
            		
            AlbumCollection list = new AlbumCollection();
            list.updateAlbumArtistCount();
            
        } finally {
            
            conn.closeConnection();
            
        }
    }
    
}
