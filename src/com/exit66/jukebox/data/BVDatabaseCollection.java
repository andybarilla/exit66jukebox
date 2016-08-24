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
import java.sql.ResultSetMetaData;
import java.sql.SQLException;
import java.util.Properties;

abstract public class BVDatabaseCollection extends BVDatabase {
    
    protected BVDatabaseObject	_currentRecord;
    ResultSet 					_rs;
    boolean 					_eof;
    int							_startRange;
    int							_offset;
    
    public void setLimit(int startRange, int offset) {
        
        _startRange = startRange;
        _offset = offset;
        
    }
    
    public BVDatabaseCollection() {
        
        _startRange = 0;
        _offset = 0;
        _eof = true;
        
    }
    
    public boolean isEOF() {
        
        return _eof;
        
    }
    
    public ResultSet retrieveData(String statement) {
        
        String limit = "";
        
        if (((_startRange == 0) && (_offset == 0)) == false) {
            
            limit = " LIMIT " + _startRange + " " + _offset + " ";
                        /* if (_offset != 0) {
                                limit .= ", " + _offset;
                        }
                        limit .= " "; */
            
            statement = statement.replaceFirst("SELECT ", "SELECT " + limit);
            
        }
        //System.out.print(statement);
        return super.retrieveData(statement);
        
    }
    
    public BVDatabaseObject getCurrent() {
        
        loadCurrent();
        return _currentRecord;
        
    }
    
    public String toJson() {
    	return toJson(0);
    }
    
    public String toJson(int childLevel) {
    	StringBuffer sb = new StringBuffer();
    	toJson(sb, childLevel);
    	return sb.toString();
    }
    
    public void toJson(StringBuffer sb) {
    	toJson(sb, 0);
    }
    
    public void toJson(StringBuffer sb, int childLevel) {
    	boolean isFirst = true;
    	
    	sb.append("[");
    	moveFirst();
    	while (!isEOF()) {
    		if (!isFirst) {
    			sb.append(",");
    		}
    		else {
    			isFirst = false;
    		}
    		getCurrent().toJson(sb, childLevel);
    		moveNext();
    	}
    	sb.append("]");
    }
    
    abstract protected void loadCurrent();
    
    public Properties getFields() {
        
        return getFields(new Properties());
        
    }
    
    
    public Properties getFields(Properties prop) {
        
        int colCount = 0;
        ResultSetMetaData rsInfo;
        
        try {
            
            rsInfo = _rs.getMetaData();
            colCount = rsInfo.getColumnCount();
            
            for (int i=1; i<=colCount; i++) {
                
                prop.setProperty(rsInfo.getColumnName(i).toLowerCase(), noNull(_rs.getString(i)));
                
            }
            
            
        } catch (SQLException e) {
            System.err.println(e);
        }
        
        return addAdditionalFields(prop);
        
    }
    
    public Properties addAdditionalFields(Properties prop) {
    	return prop;
    }
    
    public void moveFirst() {
        
        if (_rs != null) {
            
            try {
                
                if (_rs.first()) {
                    
                    _eof = false;
                    
                } else {
                    
                    _eof = true;
                    
                }
                
            } catch (SQLException e) {}
            
        }
        
    }
    
    public void moveNext() {
        
        if (_rs != null) {
            
            try {
                
                if (_rs.next()) {
                    
                    _eof = false;
                    
                } else {
                    
                    _eof = true;
                    
                }
                
            } catch (SQLException e) {};
            
        } else {
            
            _eof = true;
            
        }
        
    }
}
