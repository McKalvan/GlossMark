<%@ page contentType="text/html; charset=UTF-8" %>
<%@ taglib prefix="c" uri="http://java.sun.com/jsp/jstl/core" %>
<%@ taglib prefix="fmt" uri="http://java.sun.com/jsp/jstl/fmt" %>
<%-- Desk receipt after a checkout. Printed on the 80mm slip printer. --%>
<html>
<head><title>Checked out</title><link rel="stylesheet" href="${pageContext.request.contextPath}/css/desk.css"></head>
<body class="slip">
  <h1>Riverbend Public Library</h1>
  <p>Item <c:out value="${loan.itemId}"/> is due
     <strong><fmt:formatDate value="${loan.dueDate}" pattern="EEE d MMM yyyy"/></strong>.</p>
  <p>Renew online at riverbendlibrary.example or call 555-0142.</p>
  <form method="post" action="${pageContext.request.contextPath}/circ/checkout">
    <input name="card" autofocus> <input name="barcode"> <button>Next item</button>
  </form>
</body>
</html>
