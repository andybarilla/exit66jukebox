package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.Properties;

public class ContentType extends BVDatabaseObject {
    
    private int		_contentTypeID = -1;
    private String	_contentType;
    
    public int getContentTypeID() {
        
        return _contentTypeID;
        
    }
    
    public void setContentTypeID(int newValue) {
        
        _contentTypeID = newValue;
        
    }
    
    public String getContentType() {
        
        return _contentType;
        
    }
    
    public void setContentType(String newValue) {
        
        _contentType = newValue;
        
    }
    
    public boolean fetch() {
        
        if (openConnection()) {
            
            ResultSet rs = retrieveData("SELECT * FROM content_type WHERE content_type_id = " + _contentTypeID);
            
            try {
                
                if (rs != null && rs.next()) {
                    
                    _contentType = rs.getString("content_type");
                    
                } else {
                    
                    _contentType = "";
                    return false;
                    
                }
                
            } catch (SQLException e) {
                
                System.err.println(e);
                
            }
            
            closeConnection();
            
            return true;
            
        } else {
            
            return false;
            
        }
        
    }
    
    public void save() {
        
        ResultSet rs;
        
        if (openConnection()) {
            
            if (_contentTypeID == -1) {
                
                this.find();
                
            }
            
            if (_contentTypeID == -1) {
                
                rs = retrieveData("INSERT INTO content_type (content_type) VALUES (" +
                        Qts(_contentType) + "); CALL IDENTITY()");
                
                try {
                    
                    if (rs.next()) {
                        
                        _contentTypeID = rs.getInt(1);
                        
                    }
                    
                } catch (SQLException e) {
                    
                    System.err.println(e);
                    
                }
                
            } else {
                
                executeStatement("UPDATE content_type SET content_type = " + Qts(_contentType) +
                        " WHERE content_type_id = " + _contentTypeID);
                
            }
            
            closeConnection();
            
        }
        
    }
    
    public void remove() {
        
        if (openConnection()) {
            
            executeStatement("DELETE FROM content_type WHERE content_type_id = " + _contentTypeID);
            
            closeConnection();
        }
        
    }
    
    public boolean find() {
        
        openConnection();
        
        ResultSet rs = retrieveData("SELECT content_type_id FROM content_type WHERE content_type = " + Qts(_contentType));
        
        try {
            
            if (rs != null && rs.next()) {
                
                closeConnection();
                _contentTypeID = rs.getInt("content_type_id");
                return true;
                
            }
            
        } catch (SQLException e) {
            
            System.err.println(e);
            
        }
        
        closeConnection();
        return false;
        
    }
    
    public Properties getProperties(Properties prop) {
        
        prop.setProperty("content_type_id", String.valueOf(_contentTypeID));
        prop.setProperty("content_type", noNull(_contentType));
        
        return prop;
        
    }

    public void toJson(StringBuffer sb, int childLevel) {
    	
    }
}
